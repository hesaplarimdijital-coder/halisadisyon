module halisadisyon

go 1.24.0

toolchain go1.24.7

require (
	github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808
	github.com/ncruces/go-sqlite3 v0.32.0
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	golang.org/x/image v0.36.0
	golang.org/x/sys v0.41.0
)

require (
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/tetratelabs/wazero v1.11.0 // indirect
	golang.org/x/text v0.34.0 // indirect
)

replace (
	golang.org/x/image => github.com/golang/image v0.36.0
	golang.org/x/sys => github.com/golang/sys v0.41.0
	golang.org/x/text => github.com/golang/text v0.34.0
)
