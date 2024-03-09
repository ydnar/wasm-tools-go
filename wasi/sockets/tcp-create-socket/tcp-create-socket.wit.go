// Code generated by wit-bindgen-go. DO NOT EDIT.

//go:build !wasip1

// Package tcpcreatesocket represents the interface "wasi:sockets/tcp-create-socket@0.2.0".
package tcpcreatesocket

import (
	"github.com/ydnar/wasm-tools-go/cm"
	"github.com/ydnar/wasm-tools-go/wasi/sockets/network"
	"github.com/ydnar/wasm-tools-go/wasi/sockets/tcp"
)

// ErrorCode represents the enum "wasi:sockets/network@0.2.0#error-code".
//
// See [network.ErrorCode] for more information.
type ErrorCode = network.ErrorCode

// IPAddressFamily represents the enum "wasi:sockets/network@0.2.0#ip-address-family".
//
// See [network.IPAddressFamily] for more information.
type IPAddressFamily = network.IPAddressFamily

// Network represents the resource "wasi:sockets/network@0.2.0#network".
//
// See [network.Network] for more information.
type Network = network.Network

// TCPSocket represents the resource "wasi:sockets/tcp@0.2.0#tcp-socket".
//
// See [tcp.TCPSocket] for more information.
type TCPSocket = tcp.TCPSocket

// CreateTCPSocket represents function "wasi:sockets/tcp-create-socket@0.2.0#create-tcp-socket".
//
// Create a new TCP socket.
//
// Similar to `socket(AF_INET or AF_INET6, SOCK_STREAM, IPPROTO_TCP)` in POSIX.
// On IPv6 sockets, IPV6_V6ONLY is enabled by default and can't be configured otherwise.
//
// This function does not require a network capability handle. This is considered
// to be safe because
// at time of creation, the socket is not bound to any `network` yet. Up to the moment
// `bind`/`connect`
// is called, the socket is effectively an in-memory configuration object, unable
// to communicate with the outside world.
//
// All sockets are non-blocking. Use the wasi-poll interface to block on asynchronous
// operations.
//
// # Typical errors
// - `not-supported`:     The specified `address-family` is not supported. (EAFNOSUPPORT)
// - `new-socket-limit`:  The new socket resource could not be created because of
// a system limit. (EMFILE, ENFILE)
//
// # References
// - <https://pubs.opengroup.org/onlinepubs/9699919799/functions/socket.html>
// - <https://man7.org/linux/man-pages/man2/socket.2.html>
// - <https://learn.microsoft.com/en-us/windows/win32/api/winsock2/nf-winsock2-wsasocketw>
// - <https://man.freebsd.org/cgi/man.cgi?query=socket&sektion=2>
//
//	create-tcp-socket: func(address-family: ip-address-family) -> result<own<tcp-socket>,
//	error-code>
//
//go:nosplit
func CreateTCPSocket(addressFamily IPAddressFamily) cm.OKResult[TCPSocket, ErrorCode] {
	var result cm.OKResult[TCPSocket, ErrorCode]
	wasmimport_CreateTCPSocket(addressFamily, &result)
	return result
}

//go:wasmimport wasi:sockets/tcp-create-socket@0.2.0 create-tcp-socket
//go:noescape
func wasmimport_CreateTCPSocket(addressFamily IPAddressFamily, result *cm.OKResult[TCPSocket, ErrorCode])
