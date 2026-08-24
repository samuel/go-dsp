#!/bin/sh
# Regenerates the Parks-McClellan golden files from the Fortran original.
#
# remez.f is a lightly instrumented copy of ../remez.fortran; every change is
# marked with a "C GO-DSP:" comment.  It is built twice:
#
#   r4  stock, so REAL stays 4 bytes.  This is the historical program: its
#       grid, weights, ALPHA and impulse response are all single precision,
#       and so is the extremal search's ERR/COMP comparison.
#   r8  -fdefault-real-8 -fdefault-double-8, which widens REAL to 8 bytes
#       while holding DOUBLE PRECISION at 8.  This is what the float64 Go
#       port can be compared against exactly.
#
# -O0 matters: at higher levels gfortran may keep single-precision
# intermediates in wider registers or contract a multiply-add into an FMA,
# either of which would make the r4 output depend on the host.
set -e

cd "$(dirname "$0")"

BIN=$(mktemp -d)
trap 'rm -rf "$BIN"' EXIT

gfortran -std=legacy -w -O0 -o "$BIN/r4" remez.f
gfortran -std=legacy -w -O0 -fdefault-real-8 -fdefault-double-8 -o "$BIN/r8" remez.f

for prec in r4 r8; do
	mkdir -p "golden/$prec"
	for deck in decks/*.txt; do
		case=$(basename "$deck" .txt)
		# The driver loops until it reads a zero filter length.
		{ cat "$deck"; echo "0,0,0,0"; } | "$BIN/$prec" |
			grep '^#' > "golden/$prec/$case.txt"
		echo "$prec/$case: $(grep -c '^#H' "golden/$prec/$case.txt") taps"
	done
done
