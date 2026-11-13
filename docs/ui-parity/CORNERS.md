# Continuous corners on Windows

MyGo gates Apple continuous corners to macOS: `ui/paint.go` has
`continuousCorners = runtime.GOOS == "darwin"`. The D3D11 and OpenGL
renderers receive negative radii for continuous corners, but their shader
treats a radius of zero or less as a square corner, so enabling it on
Windows made the whole interface square.

The software raster renderer (`MYGO_GPU=0`) supports continuous corners,
but the real home page panicked in `internal/raster/raster.go` at
`drawOps` with `slice bounds out of range [167040:0]`.

So the patch was reverted. The app uses circular corners with a larger
scale until continuous corners are implemented in the D3D11/OpenGL
shaders.
