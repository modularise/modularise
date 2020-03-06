# Rationale

This document intents to provide a high-level explanation of the what, why & how that led to the
`modularise` tool being built. For an in-depth technical design please see [this document].

[this document]: ./technical_breakdown.md

## Context

Many existing larger & monolithic Go projects face the issue, or have faced it, of [adopting
modules] as part of their day-to-day workflow and project management. One of the major issues that
they face is having multiple levels of APIs:

* A system-level API that supports and delivers the core features of the project.
* One or more inner-level APIs that are used to implement the system-level API without being part of
  it, or exposed by it,.

Many of these projects tend to base their semantic versioning on their system-level API, and use it
to decide on whether a new release should be a patch, minor or major version change. This does
however not cover the fact that the inner-level API(s) will evolve on an entirely different timeline
and cadence. As a result the project does not properly follow the requirements of semantic
versioning with breaking changes in the inner APIs happening between minor or even patch releases.

The typical “Go” solution would consist in making these inner-level APIs part of an [`internal`
tree] that is then used by the system-level APIs implementation. However this approach has not
historically been followed by most of these larger projects, and might not make sense for advanced
users that need to access the inner-level APIs in their own downstream projects. Entire open-source
ecosystems have grown to depend on the inner-level APIs of these larger projects. Hence it is no
longer a viable option from a community perspective to make them internal.

Another typical approach, based on software engineering best-practices, would be to split out the
packages of the inner-level APIs into separate projects that can then follow their own versioning
strategy, with the system-level API’s implementation simply depending on these new external
projects. This option has however not been found viable by many projects as it drastically reduces
feature velocity and significantly increases the barrier for new developers to contribute as the
work for a single feature would be split across multiple repositories.

A third solution, provided by Go modules themselves, is to make use of a [multi-module repository].
This does however also come with some significant pain points and is explicitly [recommended for
power-users only]. Even then, considering that larger projects can be considered to be power-users,
this approach comes with multiple pain-points and [complex infrastructure].

The `modularise` command-line tool that is proposed in this document aims to provide a smooth
module-based development experience for contributors, maintainers and downstream consumers of such
larger, monolithic Go projects. It addresses some of the key issues that such codebases encounter
when adopting and maintaining Go modules by providing automation of some of the underlying
maintenance tasks. The resulting maintenance cost and complexity should be significantly lower than
what would be required for a multi-module setup. As a secondary benefit it provides a clean, two-way
migration path between monolithic and more segmented project setups.

[adopting modules]: https://groups.google.com/forum/#!topic/prometheus-developers/F1Vp0rLk3TQ
[`internal` tree]: https://golang.org/doc/go1.4#internalpackages
[multi-module repository]: https://github.com/golang/go/wiki/Modules#faqs--multi-module-repositories
[recommended for power-users only]: https://github.com/golang/go/wiki/Modules#should-i-have-multiple-modules-in-a-single-repository
[complex infrastructure]: https://stupefied-goodall-e282f7.netlify.com/sigs-and-wgs/contributors/devel/staging/

## Goals

1. Enable the seamless and automatic setup and maintenance of multiple Go modules for which the
   source code is contained in a single codebase / repository.
2. Automate the maintenance of diverging versioning timelines of multiple Go modules for which the
   source code is contained in a single codebase / repository.
3. Enable a simple two-way migration path for Go libraries between being an integral part of a
   larger codebase and being an independent stand-alone project.

## High-Level Overview

The `modularise` tool can be used both manually or as part of an automated setup (continuous
integration, ...). Its responsibility is to carve libraries, or other sub-projects, out of a larger
project and use these to populate and maintain side-repositories. The mapping between the various
pieces of the projects and these side-repositories is entirely configurable.

Each of the side-repositories managed by modularise is a fully independent Go module with its own
version timeline maintained in accordance to semantic versioning. Downstream consumers of the
libraries that `modularise` splits out of the project will need to depend on these side-repositories
instead of on the original packages in the project itself.

The project should move all the libraries split-out by `modularise` into an `internal` tree so that
they are no longer exposed to downstream consumers to further reduce the potential for confusion.
This also has the effect of reducing the project’s exported API surface which in turn reduces the
difficulty of maintaining an appropriate semantic versioning strategy.
