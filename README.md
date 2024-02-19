# tmux-vcs-sync
Synchronize the underlying VCS based on tmux state.

## What is this?

tmux-vcs-sync is a developer tool that I've had in some form basically since I
started working. At its core, it's trying to automate some tedious bookkeeping
that I would otherwise need to do for my normal development workflow. But that
tedious bookkeeping is pretty easy to forget, so there's a bit of a productivity
boost here, too.

My developer workflow is essentially a [stacked diff workflow](https://graphite.dev/guides/stacked-diffs).
Each of my work units is a small, self-contained change. If the feature I'm
working on isn't done after a single, small, self-contained change, I start
another based on top of the work I've already done. I find that this workflow
has a ton of benefits (more incremental progress, easier to verify each
individual change, changes that are less "dramatic"), but it can also be harder
to verify the full effects of a series of changes when they're all separate, so
it has some tradeoffs.

I found it a little difficult to keep track of all my small changes (I don't
think humans are really meant to be able to remember a bunch of 160 bit hex
hashes), so I tweaked my workflow a bit to make this a little easier.

1. I started naming all of my work units.

   The names weren't always great (naming is hard), but they did help me
remember what I was working on in each change.

2. I started using a separate tmux session per work unit.

   I found this helped reduce the cost of context switching. Maybe it has
something to do with each change having its own "workplace", and me seeing the
last couple of commands I ran when working on the change primes my memory.

But then I found that I was switching into tmux sessions and forgetting to
update the VCS to checkout the correct work unit. It feels pretty bad to have to
unroll dozens of minutes worth of work because of a small oversight.

And that's where this tool comes in. I named all of my tmux sessions the same as
my work units. The first iteration of this tool was some hacky bash functions
combined with [bash-preexec](https://github.com/rcaloras/bash-preexec) to ensure
the VCS was updated before any command ran in my terminal. It has since grown
beyond just some bash functions, and I thought maybe someone else would benefit
if I put it out into the wild :)

## Installation

This project uses [mage](https://magefile.org/). See that project's website for
installation instructions.

```sh
$ git clone https://github.com/JeffFaer/tmux-vcs-sync
$ cd tmux-vcs-sync
$ mage install
$ cd git
$ mage install
```

### Shell completion

You can generate shell completion with

```sh
$ mage install:completion "${SHELL:?}"
```

where `${SHELL:?}` is one of the shells that
[cobra](https://github.com/spf13/cobra) is able to generate
completion for (at the time of writing: bash, fish, powershell, and zsh).

It will generate a `tvs_completion.${SHELL:?}` file and attempt to put it into
an appropriate directory for your shell.

## Usage

```sh
$ tmux-vcs-sync new work-unit-name
$ tmux-vcs-sync commit work-unit-name
$ tmux-vcs-sync rename work-unit-name
$ tmux-vcs-sync update work-unit-name
$ tmux-vcs-sync update
```

  - `new work-unit-name`: Create a new tmux session and a new work unit on the
    current repository's trunk.
  - `commit work-unit-name`: Create a new tmux session and a new work unit on
    top of the current work unit.
  - `rename work-unit-name`: Rename the current tmux session and work unit.
  - `update work-unit-name`: Update tmux and the underlying VCS to both point at
    the given work unit.
  - `update`: Update the repository of the current tmux session to point at the
    tmux session's work unit.

This information and more can be found in the tool itself:

```sh
$ tmux-vcs-sync help
```

## Tips

### `tmux-vcs-sync`? That's a lot to type.

Yeah, I know. Naming is hard. Consider aliasing it to `tvs`. Or pitch me a
better name that's easier to type :)

### How do I make it run before terminal commands?

1. Install [bash-preexec](https://github.com/rcaloras/bash-preexec).
2. Add the following snippet to your .bashrc.

   ```sh
   if [[ -n "${TMUX}" ]]; then
     tvs_preexec() {
       if ! git ls-files --error-unmatch &>/dev/null; then
         return
       fi
       if tmux-vcs-sync update --fail-noop; then
         history -s tmux-vcs-sync update
       fi
     }
     preexec_functions+=( "tvs_preexec" )
   fi
   ```

## Development

This is a multi-module project.

<!--
https://tree.nathanfriend.io/?s=(%27opFs!(%27fancy!true~fullPTh9~trailingSlash9~rootDot9)~K(%27K%27tJ-vcs-sync3.0th6gi8roo8isQ6ReQT2sQ6CLI4oolOcmd0cobra%20commandsOtJBt4JUPIO*stTe0GsEom6operaFs4oEync4J7nd%20VCSEtTe3api5UPIQa8can%20be2ed4o7ddEuppor8for7%20new%20VCSOexecB8wrapper7roundHs%2FexecOMesO*plugin0A%20M6libraryN2aFsHfQ6api3git52aFHf7piN%20git%27)~version!%271%27)*%20%200%20--%202%20G3%5Cn*4%20t50this%20R6is7n6e%207%20a8t%209!falseB0lightweighE%20sFtionGimplementH%20oJmuxKsource!MmagefilN%20forO3*Q4hRmodulTatU%20A%01UTRQONMKJHGFEB987654320*
-->
```
tmux-vcs-sync
├── . -- the git root is the module that implements the CLI tool
│   ├── cmd -- cobra commands
│   └── tmux -- lightweight tmux API
│       └── state -- implements some operations to sync tmux and VCS state
├── api -- this module is an API that can be implemented to add support for a new VCS
│   ├── exec -- lightweight wrapper around os/exec
│   └── magefiles
│       └── plugin -- A magefile library for implementations of the api
└── git -- this module is an implementation of api for git
```

You will probably want to set up go.work when developing locally:

```sh
$ go work init
$ go work use . api git
```

### TODOs

  - Tests.
  - Update should accept repo-qualified work unit names.
  - tmux display-menu with better work unit ordering.
  - tmux hooks to automatically update session names when a session closes.
