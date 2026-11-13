param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Rest)
# The project builds with Go 1.27.3, as go.mod says. Go downloads that
# toolchain if the machine has none or an older one, and uses it even when
# the machine has a newer one: the same build everywhere.
$env:GOTOOLCHAIN = "go1.27.3"
& go tool mygo @Rest
exit $LASTEXITCODE
