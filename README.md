# lazyreview

Terminal UI to review GitHub pull requests: browse PRs, read whole files annotated
with their diff, mark files as viewed (synced with GitHub), add line or range
comments to a pending review and submit it.

![lazyreview demo](demo/demo.gif)

## Install

```sh
mise install            # Go toolchain pinned in mise.toml
mise run build          # builds ./lazyreview
```

Authentication uses `GITHUB_TOKEN` / `GH_TOKEN`, or falls back to `gh auth token`.

## Usage

```sh
lazyreview                         # repository from the origin remote of the cwd
lazyreview --repo owner/name       # explicit repository
lazyreview --repo owner/name --pr 42
lazyreview --theme dracula         # any chroma style
```

On restart, the last PR, file and line are restored. PR lists, PR details and
file contents are cached under `~/.cache/lazyreview/<owner>/<name>/` and shown
immediately, then refreshed in the background (edits wait for the refresh).
Unused entries are pruned after 30 days.

Press `?` for all keys. The essentials:

| Key | Action |
| --- | --- |
| `tab` `h` `l` | switch pane |
| `ctrl+h/j/k/l` | move to pane left / down / up / right |
| `+` `_` | enlarge / shrink the focused pane (normal → half → full) |
| `enter` | open PR / file |
| `space` | toggle file viewed, then jump to next unviewed file |
| `]` `[` | next / previous file |
| `n` `N` | next / previous change |
| `d` | full file ↔ hunks only |
| `v` then `c` | comment a range (`c` alone comments the cursor line) |
| `x` | delete the pending comment under the cursor |
| `e` | open the file at the cursor line in `$EDITOR` (or the parent Neovim) |
| `S` | submit review (Comment / Approve / Request changes) |

In comment dialogs, `ctrl+s` confirms and `ctrl+e` opens `$EDITOR`.

## Neovim / LazyVim

Like lazygit, lazyreview can run in a floating terminal. When started inside
Neovim (`$NVIM` is set), `e` hides the float and opens the file in the parent
Neovim; toggling the float again resumes the same session.

```lua
-- ~/.config/nvim/lua/plugins/lazyreview.lua
return {
  "folke/snacks.nvim",
  keys = {
    {
      "<leader>gv",
      function()
        Snacks.terminal("lazyreview", { cwd = LazyVim.root.git(), win = { style = "lazygit" } })
      end,
      desc = "LazyReview",
    },
  },
}
```

## Tests

```sh
mise run test
LAZYREVIEW_TEST_REPO=owner/name go test -tags integration ./internal/gh/   # read-only, real API
```

## Demo

`demo/demo.gif` is recorded with [VHS](https://github.com/charmbracelet/vhs)
against the sandbox repository
[adelplace/lazyreview-demo](https://github.com/adelplace/lazyreview-demo):

```sh
sudo pacman -S vhs ttyd   # or see the VHS install instructions
demo/setup.sh             # once: creates the sandbox repository and its PRs
mise run demo             # resets the PRs, then records demo/demo.tape
```
