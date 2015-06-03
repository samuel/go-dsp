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

### Hardware Interfaces

* Wrapper for librtlsdr

### Clients & Servers

* borip compatible client and server
