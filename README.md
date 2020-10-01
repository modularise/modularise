# Modularise

The `modularise` tool helps you maintain best-practices for [Go modules] on large-scale projects. It
addresses the issues of monolithic codebases that contain many separate feature-sets or libraries
that each require a separate release schedule in order to comply with the [Semantic Versioning]
policy which underpins the [Minimal Version Selection] (MVS) algorithm used by Go modules.

Although a naïve solution to such a situation would be to use [a multi-module repository], this
approach bears considerable risks and has many unintendend, unexpected and undesirable drawbacks. A
better solution is to split out the various feature-sets into their own dedicated Go modules in
separate version-controlled repositories. This however has the drawback that it becomes impossible
for developers to make simultaneous changes in different repositories, thereby reducing the speed
and flexibilty of their development workflows.

Enter `modularise` which allows project to benefit from the best of both worlds:

1. A single repository containing the entire source code for their project, with all the benefits of
    the speed and flexibility that such a setup offers for feature development.
2. Independently versioned Go modules for each of the various independent feature-sets that their
    projects offers to downstream users, thereby reducing the burden of version and release
    management on project maintainers.

For more details about the context that drove the creation of `modularise` you can refer to the
dedicated [rationale document].

[Go modules]: https://blog.golang.org/versioning-proposal
[Semantic Versioning]: https://semver.org
[Minimal Version Selection]: https://research.swtch.com/vgo-mvs
[a multi-module repository]: https://github.com/golang/go/wiki/Modules#should-i-have-multiple-modules-in-a-single-repository
[rationale document]: ./docs/design/rationale.md

## Example setup

### Walk-Through

Consider a Go project that has the following (simplified) package structure:

```text
.
├── go.mod
├── internal
│   └── random
│       └── generate.go
├── main.go
├── numberutils
│   └── number.go
└── stringutils
    └── string.go
```

The core API of this project is the one provided by the app built by the top-level `main` package.
However other Go projects might want to use either the `numberutils` and `stringutils` packages that
provide a considerably amount of reusable functionality.

For the project maintainers, who need to comform to the requirements of semantic versioning, this
situation could quickly become problematic as the API evolutions of the app and the two libraries
might very well follow completely different timelines. Given that the app is their main product it
would be very strange to release a new major version of the app just because a breaking change was
made in either of the libraries.

With `modularise` the directory layout would be modified to stop exposing the libraries as part of
the project's public API and become:

```text
.
├── go.mod
├── internal
│   ├── numberutils
│   │   └── number.go
│   ├── random
│   │   └── generate.go
│   └── stringutils
│       └── string.go
├── main.go
└── modularise.yaml
```

At this point the [configuration] in `modularise.yaml` allows the `modularise` tool to automatically construct and maintain two separate repositories with the following content:

```text
.
├── go.mod
├── internal
│   └── random
│       └── generate.go
└── stringutils
    └── string.go
```

and

```text
.
├── go.mod
├── internal
│   └── random
│       └── generate.go
└── numberutils
    └── number.go
```

These independent repositories also implement two independent Go modules that can then be tagged and versioned at their own rythm without affecting the releases of the original project's app.

[configuration]: #example-configuration

### Example Configuration

The configuration in `modularise.yaml` in the example shown above would simply be:

```yaml
credentials:
  # Authentication details to push to the target repositories
splits:
  numberutils:
    module_path: github.com/me/numberutils
    url: git@github.com:me/numberutils
    includes:
      - internal/numberutils
  stringutils:
    module_path: github.com/me/stringutils
    url: git@github.com:me/stringutils
    includes:
      - internal/stringutils
```

## Configuration Reference

The configuration format is fully documented [in code] via YAML annotations on the `Splits` type and
the types of the fields that it references.

An exhaustive example configuration can also be found below:

```yaml
credentials:
  # Specify one of the three credential options
  pub_key: ./auth/id_rsa
  token_envvar: TOKEN_VAR
  userpass:
    username: robot
    password_file: ./auth/pwd
author:
  # If no author information is provided a default modularise identity is used
  name: CI robot
  email: robot@company.org
splits:
  client:
    module_path: company.org/client
    # If no URL is set the split will be available locally
    url: git@ssh.company.org:repos/client
    includes:
      - cmd/client
  server:
    module_path: company.org/server
    url: git@ssh.company.org:repos/server
    # The used branch defaults to 'master' if not set
    branch: modularise
    includes:
      - cmd/controller
      - cmd/server
    excludes:
      - cmd/server/config
```

[in code]: ./cmd/config/splits.go

## Best Practices

### API surface

A Go module should generally expose a coherent set of features. Any piece of public API is a form of
liability for the project's owners as it may induce maintenance and support burden. Furthermore the larger the API surface of a project, the more frequent breaking changes are likely to occur. As a
result developers are encouraged to keep their API surfaces to the minimum. The use of [`internal`]
package trees is strongly encouraged in order to limit the visibility of packages that provide the
internal implementation logic of their libraries.

If a project would like to provide some of its internal libraries as a separate & dedicated API to
downstream users this would typicallly be done by creating a new split with `modularise` that
contains the packages with the API that should be exposed.

[`internal`]: https://golang.org/doc/go1.4#internalpackages

### Continuous Integration

The default set up for a project using `modularise` would see the tool run in _dry-run_ mode on each
pull request before it is merged to ensure that the split configuration remains valid.

A second continuous integration job should run on every push to the project's `master` branch
without the `--dry-run` flag in order to update the content of all configured splits with the latest
version of the core project.

### Semantic Versioning Of Splits

The `modularise` tool, although it maintains the content of all the configured split repositories,
does not perform any form of version tagging. It is up to the core project's maintainers to decide
when to release new versions of each split. Releases for different splits can be done at different
times.

If a change in the core project's source-code results in a breaking change in the API of a split,
the configured module path should be updated to reflect the new major version of the split in
question. This can even be done in hindsight as there is no guarantee of backwards compatibility
between pseudo-versions with Go modules. Such guarantees only exist for tagged releases and these
are on purpose done manually.

## FAQ

* **How does `modularise` work internally?**

  You can read the [technical design document] for `modularise` to find out.

  [technical design document]: ./docs/design/technical_breakdown.md

* **Does `modularise` enable two-way migrations between splits and independent projects?**

  A project can decide to turn one of its configured splits into an independently maintained and
  developer repository. This would be done by removing the split's code from the core project and
  having the core project depend on the split's repository instead. That repository can then from
  that point on be managed as any other Go project.

  The reverse is possible too. For example consider a third-party project that is very close to the
  core one, either because of a dependency or because of the features it implements. This project
  might over time stop being maintained for arbitrary reasons or it might decide to merge with the
  core project for organisational reasons. The core project can obviously decide to take over its
  active development as a separate repository. However, if the two projects are effectively being
  merged at the source-code level, they can also copy this source code into the `internal` tree of
  the core project. This new sub-tree would then be set up as a split pointing to the former
  repository. At this point contributions will be taken via the core project which will mirror
  changes to the former repository via `modularise`.

* **Can’t we apply the same type of logic implemented by `modularise` to facilitate the use of
  nested modules?**

  This is definitely possible. There are however a few advantages to `modularise`’s approach:

  * The code of splits can be easily downloaded & consulted in a standalone fashion as it is stored
    in a separate repository. For nested modules the code is contained in the same repository as all
    other modules and it is up to the person navigating through the code to keep in mind a mental
    model of the boundaries between the modules.
  * The mapping of content to a split is more flexible than what nested modules allow. Imagine the
    example path `/root/foo/bar` in the core project. One can imagine that the project might want to
    export the `foo/` subtree as a module, with the exception of the `bar/` child-subtree. This is
    not possible with nested modules as a module rooted at foo/ would automatically contain `bar/`
    as well. With modularise the setup can be such that `bar/` is not part of the split of `lib/`.
