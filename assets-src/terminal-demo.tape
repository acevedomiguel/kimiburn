# Kimiburn terminal demo capture for the README and the project website.
#
# Run from the repository root after `make build`:
#   vhs assets-src/terminal-demo.tape
#
# The demo command renders the synthetic fixture from internal/ui/demo.go; no
# token, configuration, cache, or network access is involved. Every value in
# the capture is fictional.

Set Shell bash
Set FontSize 16
Set FontFamily "Menlo"
Set Width 1400
Set Height 440
Set Padding 24
Set TypingSpeed 50ms

# The recorder inherits the operator's environment; a global NO_COLOR would
# force the dashboard into its monochrome mode, which is not representative.
Type "clear && env -u NO_COLOR ./kimiburn demo"
Enter

# Let the meters finish their opening sweep before the capture.
Sleep 3s

Screenshot assets-src/terminal-demo.png

Type "q"
Sleep 500ms
