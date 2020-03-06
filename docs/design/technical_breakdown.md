# Technical Design

For reference the original, somewhat outdated, version of this document can be found [here]. The
content below is condensed and simplified based on the experience and learnings acquired from
implementing the logic that was initially planned.

[here]: https://docs.google.com/document/d/1g7wIlxn9JBJkc-2lHAb2MfCYLltWZKyNlIO6SBW1x-8/edit#

## Glossary

The following glossary elaborates on a few technical terms used in the remainder of this design
document. You can read them for starters while lacking context, or refer back to this list at a
later stage if and when needed.

### Go Module

A set of Go packages that together implement one or more functionalities with a varying degree of
coupling between them. Go modules are a versioning abstraction, compared to packages which are a
feature-set abstraction. A module is the atom of the dependency resolution performed by the Go
toolchain. More information can be found on the [Go Wiki] and in the release-notes of Go [1.11],
[1.12], [1.13] & [1.14].

[Go Wiki]: https://github.com/golang/go/wiki/modules
[1.11]: https://golang.org/doc/go1.11#modules
[1.12]: https://golang.org/doc/go1.12#modules
[1.13]: https://golang.org/doc/go1.13#modules
[1.14]: https://golang.org/doc/go1.14#introduction

### Core Project

Top-level project out of which the `modularise` tool will carve independent Go modules. This would
typically be a larger project that currently exposes system-level APIs as well as implementation
libraries in a single monolithic codebase, be it without yet having adopted modules, by using a
single module or a multi-module repository.

Examples include [Prometheus], [Thanos], [Cortex], [Kubernetes], [Moby], and many private corporate
codebases.

[Prometheus]: https://github.com/prometheus/prometheus
[Thanos]: https://github.com/thanos-io/thanos
[Cortex]: https://github.com/cortexproject/cortex
[Kubernetes]: https://github.com/kubernetes/kubernetes
[Moby]: https://github.com/moby/moby

### Split

A repository, separate from the core project’s repository, managed automatically via the
`modularise` tool, and which contains a sub-set of the Go packages of the core project. The
repository is itself a single Go module.

### Residual

A package of the [core project] that is not part of any split but is imported by another package
that is part of a split. The package should only be referenced by the internal implementation of the
second package and may not be the source of a type that is used within the second package's public
API.

[core project]: #core-project

## Creating Splits

Although `modularise` will need to analyse the core project's code and potentially rewrite parts of
it, those rewrites are limited to import path modifications. The actual source code of the core
project's packages are never modified in any functional way.

The process of creating one or more splits should follow a well-defined set of steps:

1. **Parsing & Loading**

    Parse all the core project’s directories and files and determine, based on the provided split
    configuration which directories and corresponding Go packages should be part of which split.

2. **Residual Analysis**

    For each configured split perform the following analysis:

    1. Iterate over all the split's Go packages and register any imports of packages that are part
        of other splits as a dependency between the two splits.
    2. Register imported packages that are not part of any split as residuals of the current split.
        Recurse this analysis on the packages that were just marked as residuals until a stable set
        of residual packages has been achieved.
    3. Iterate over the exported symbols of all the split's non-residual Go packages and ensure that
        the types that they reference are defined by either the split itself or another split on
        which it depends.

    Verify that there are no cycles within the graph of dependency's between splits.

3. **Split Generation**

    For each configured split do the following:

    1. Initialise a working directory for the split. Two variants, depending on whether we are
        creating a brand new split or maintaining an existing split's repository:
        - Create a new directory and initialise it as a git repository.
        - Clone the exising repository of the split and clean out it's current content.
    2. Considering all directories that are part of the split, either via explicit configuration or
        as residuals of the split, compute the longest common path prefix within the core project.
        This path is the split's virtual root.
    3. Copy over the content of all directories that are part of the split via explicit
        configuration. The path of a file within the split's working directory should correspond to
        the path of the file in the core project relative to the split's virtual root.
    4. Copy over the content of all the residual packages to the split's working directory. If the
        package's path relative the virtual root contains an `internal` component then the new path
        will simply be this same relative path. If no such component exists then the new path should
        be prefixed by an `internal` component so that the copied package will not be part of the
        split's public API.
    5. For all Go packages, residual or not, within the split's working directory rewrite any import
        paths referencing packages that are part of the split or dependent splits to use their new
        paths.

4. **Dependency Maintenance**

    Traverse all configured splits by following a depth-first traversal of the graph of dependencies
    between splits. For each split:

    1. Copy over the `go.mod` file of the core project into the root of the split's working
        directory. Modify the path of the file's `module` statement to reflect the split's new
        module path.
    2. For each dependency on another split add a `replace` statement for the target split's module
        path targeting that split's working directory. This ensures that the split's working
        directory now represents an independent & buildable Go module.
    3. Correct any missing or superfluous `require` statements by running the `go mod tidy` command
        within the split's working directory.
    4. Remove the previously introduced local `replace` statements and edit the `require`
        statements for each split dependency to reference that split's freshly determined
        pseudo-version

        _NB: this is why we iterate through the splits via a depth-first traversal of the dependency
        graph._

    5. Register the content as a new commit in the git repository of the split's working directory.
    6. Compute the new pseudo-version for the split's module based on existing git history, module
        path and the hash of the new commit.

    If all split's were correctly generated, push the new content to all their respective git
    remotes.

## Residuals: What & Why

A [split] is defined by the packages from the core project that it will include. The split's public
API is the set of all symbols exported by the split's packages. This API must be defines such that
it is usable, interoperable with other splits from the same core project and such that the split has
a well-defined Go module dependency graph.

[split]: #split

### Self-Contained APIs

The public API of a split should be "self-contained". This corresponds to all the types referenced being themselves defined by a package belonging to the split, or to a package part of another split
on which the first one can depend.

As a counter example, assume that we allowed the public API of a split to reference a package that
is not part of any split. Any project depending on the split's module would then also implicitly
depend on the core project's module. This defeats the purpose of a split which is to provide a
sub-set of the features of the core project without having to depend on the latter.

### Cross-Split Interoperability

Even if the public API of a split may be "self-contained", the packages implementing this API may
depend on a much larger set of packages that are not themselves part of the split. These packages
that are not part of a split but are used by the split's internal implementation are named residuals
of the split.

Residual packages may not by referenced by the split's public API as this would allow for type
conflicts to appear. Consider a package `p` that is a residual of both split A and split B. If the
content of `p` were to be copied over to the working directory of both A and B then there would now
be two definitions for each of the types defined by `residual`.

If, for example, a function in A would return an object of a type defined by `p` then a project using both A and B would no longer be able to pass along the returned object into a function defined
by B consuming the same type as the two functions would now refer two distinct types.

## Well-Defined Split Dependencies

Although one split may depend on another, cycles within the resulting dependency graph are not
allowed. This is to prevent the formation of infinite dependency cycles within the resuling module
dependency graph.

Consider a hypothetical cycle involving two inter-dependent splits A and B. When generating the
content of both A and B from the same commit in the core project then the `go.mod` file of A would
need to reference the newly generated pseudo-version of B and the `go.mod` for B would in turn need
to reference the newly generated pseudo-version for A. However the `go.mod` file of A is itself
part of the commit that defines this pseudo-version that should be referenced by B, which influences
the content of the `go.mod` of B which in turn defines the pseudo-version that A should reference.
