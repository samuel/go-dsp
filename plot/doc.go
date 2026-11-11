// Package plot draws the results of the analysis package: line charts with
// labeled axes, spectrograms with a color bar, and the palettes behind them.
//
// It depends on nothing outside the standard library. A Chart writes itself as
// SVG; a spectrogram's data pane is a PNG embedded in that SVG, so a labeled
// raster needs no plotting library. Pixel coordinates go through lerp in
// svg.go so the output is byte-identical on every architecture.
package plot
