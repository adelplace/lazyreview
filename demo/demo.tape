# Records demo/demo.gif. Run from the repository root with `mise run demo`.

Output demo/demo.gif

Set Shell bash
Set Width 1400
Set Height 800
Set FontSize 16
Set Padding 12
Set Theme "Catppuccin Mocha"
Set TypingSpeed 80ms
Set Framerate 24

# Fresh cache and session so the demo always starts the same way.
Hide
Type "export PATH=$PWD:$PATH XDG_CACHE_HOME=$(mktemp -d) && clear"
Enter
Show

Type "lazyreview --repo adelplace/lazyreview-demo"
Sleep 500ms
Enter
Sleep 3s

# Filter the PR list and open a pull request.
Type "/pag"
Sleep 800ms
Enter
Sleep 400ms
Enter
Sleep 3s

# Browse files: the diff previews the file under the cursor.
Down
Sleep 1s
Down
Sleep 1s
Up
Sleep 800ms
Enter
Sleep 1s

# Jump between changes, then toggle hunks only / full file.
Type "n"
Sleep 800ms
Type "n"
Sleep 800ms
Type "d"
Sleep 2s
Type "d"
Sleep 1s

# Comment a range of lines.
Type "v"
Sleep 300ms
Down 3
Sleep 600ms
Type "c"
Sleep 800ms
Type "Should we cap the page size here too?"
Sleep 600ms
Ctrl+S
Sleep 2500ms

# Enlarge the diff, then the file list.
Type "+"
Sleep 1500ms
Type "_"
Sleep 800ms
Tab
Tab
Sleep 500ms
Type "+"
Sleep 1500ms
Type "_"
Sleep 800ms

# Mark the file viewed: the next unviewed file opens.
Space
Sleep 2s

# Review submission dialog (cancelled).
Type "S"
Sleep 2500ms
Escape
Sleep 800ms

Type "?"
Sleep 3s
Escape
Sleep 500ms
Type "q"
Sleep 500ms
