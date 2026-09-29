# lazyreviewer

Terminal UI to review GitHub pull requests: browse PRs, read whole files annotated
with their diff, mark files as viewed (synced with GitHub), add line or range
comments to a pending review and submit it.

## Install

```sh
mise install            # Go toolchain pinned in mise.toml
mise run build          # builds ./lazyreviewer
```

Authentication uses `GITHUB_TOKEN` / `GH_TOKEN`, or falls back to `gh auth token`.

## Usage

```sh
lazyreviewer                         # repository from the origin remote of the cwd
lazyreviewer --repo owner/name       # explicit repository
lazyreviewer --repo owner/name --pr 42
lazyreviewer --theme dracula         # any chroma style
```

Press `?` for all keys. The essentials:

| Key | Action |
| --- | --- |
| `tab` `h` `l` | switch pane |
| `ctrl+h/j/k/l` | move to pane left / down / up / right |
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

Like lazygit, lazyreviewer can run in a floating terminal. When started inside
Neovim (`$NVIM` is set), `e` hides the float and opens the file in the parent
Neovim; toggling the float again resumes the same session.

```lua
-- ~/.config/nvim/lua/plugins/lazyreviewer.lua
return {
  "folke/snacks.nvim",
  keys = {
    {
      "<leader>gv",
      function()
        Snacks.terminal("lazyreviewer", { cwd = LazyVim.root.git(), win = { style = "lazygit" } })
      end,
      desc = "LazyReviewer",
    },
  },
}
```

## Tests

```sh
mise run test
LAZYREVIEWER_TEST_REPO=owner/name go test -tags integration ./internal/gh/   # read-only, real API
```
