# Renders docs/demo.gif with charmbracelet/vhs: https://github.com/charmbracelet/vhs
#
#   make demo        (needs vhs, plus ttyd and ffmpeg: brew install vhs)
#
# Four commands on examples/demo-app, about 15 seconds. The recording runs the
# real binary built by `make build`, so the GIF always shows current behavior.

Output docs/demo.gif

Require go

Set Shell "bash"
Set FontSize 15
Set Width 1180
Set Height 760
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set PlaybackSpeed 1

# Setup the viewer never sees: put the built binary on PATH, enter the demo.
Hide
Type "export PATH=$PWD/bin:$PATH PS1='$ ' && cd examples/demo-app && clear"
Enter
Show

# 1. `envguard` = doctor: the full report (the screen ends on the matrix and next steps).
Type "envguard"
Sleep 400ms
Enter
Sleep 3s

# 2. The compact CI view.
Type "clear && envguard check"
Sleep 300ms
Enter
Sleep 3s

# 3. Why a rule fires.
Type "clear && envguard explain EG003"
Sleep 300ms
Enter
Sleep 3s

# 4. Where staging and production disagree.
Type "clear && envguard diff staging production"
Sleep 300ms
Enter
Sleep 3s
