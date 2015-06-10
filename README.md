Software Defined Radio (SDR) package and tools for Go
-----------------------------------------------------

This repo is a collection of packages and tools for working with SDR in Go.

It also includes ARM assembly optimized filters and conversions which
allow real-time FM demodulation on the Raspberry Pi.

### Demodulators

* FM (polar disciminator)
* AFSK

### Decoders

* AX.25
* DTMF

### Other Algorithms

* Goertzel
* Sliding DFT

### Clients & Servers

* borip compatible client and server

## Filter design

Go packages for filter design:

- [Parks-McClellan aka Remez](https://github.com/samuel/go-remez)

## SDR Hardware Interfaces

Go packages to utilize SDR hardware:

- [RTL-SDR](https://github.com/samuel/go-rtlsdr)
- [HackRF](https://github.com/samuel/go-hackrf)
