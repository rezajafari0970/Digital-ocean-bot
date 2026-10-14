# Own-socket TCP observation

This experimental diagnostic utility reads Linux/Android TCP_INFO from a socket owned by the calling process. It does not capture other applications, change routing, modify v2rayNG or Xray, tune the kernel, or provide a relay.

Compile tcp_info_reader.c as a shared library using the headers and compiler on the device where it will run. TcpInfo.snapshot(socket, label) accepts an existing socket. The only CLI operation in tcp_info_reader.py is --self-test, restricted to loopback. There is deliberately no arbitrary remote-target CLI and no lifecycle/host-key bypass.

Example for an operator-controlled scratch directory:

```sh
cc -shared -fPIC -O2 -Wall -Wextra -Werror tcp_info_reader.c -o /private/scratch/libtcpdiag.so
python3 tcp_info_reader.py --library /private/scratch/libtcpdiag.so --self-test
```

Use a distinct native library basename such as libtcpdiag.so. Do not place tcp_info_reader.so beside tcp_info_reader.py: Python may select the shared object as an extension module, although this library exposes a ctypes C ABI and has no PyInit entry point.

The C reader uses the native header's offsetof and field size, and the returned getsockopt length. Unsupported/truncated fields are null, never invented zero values. No Python hard-coded ABI offsets are used. Keep library/header/source hashes, architecture, kernel version and UTC/monotonic timestamps in the measurement receipt. Native architecture binaries and private receipts are not committed.

Validation on 2026-10-14:
- Android aarch64 6.1.138: four loopback nonce/hash/body/counter cases passed; TCP_INFO returned 232 bytes.
- Manager Linux: the same four cases passed; returned size 280 bytes.
- Closed descriptors correctly produced EBADF on both devices.
- Termux AF_PACKET/SOCK_RAW creation was denied (errno 13); no raw packets were collected.
- This validates instrumentation only. It is not a new target, carrier, v2rayNG or production transport test.

Interpretation boundaries:
- tcpi_bytes_received counts socket-level TCP bytes, not a particular TLS plaintext body.
- tcpi_bytes_acked can include the SYN sequence-space byte; the self-test observed upload length plus one.
- segs_out minus data_segs_out is not an exact ACK counter; it can include SYN/FIN/RST and other non-payload segments.
- In the reference Android common 6.1 TCP output code, segs_out is incremented before queue_xmit. Increasing counters therefore do not prove physical modem transmission or peer reception. Vendor source may differ.
- Offload, socket close timing and sample timing affect interpretation.
- A nonzero output counter plus a missing server packet only narrows the observation interval; it cannot name the intervening device or distinguish intentional DPI from every firewall, NAT, routing or delivery failure.

Before any non-loopback use, the enclosing experiment must enforce existing target identity, account/provider state, lifecycle and pinned-host guards. Pair snapshots with a bounded server capture and a nonce-correlated request; verify source ASN and direct routing. Do not infer an earlier experiment's phone counters from a later self-test.

References:
- https://man7.org/linux/man-pages/man7/tcp.7.html
- https://android.googlesource.com/kernel/common/+/refs/heads/android14-6.1/net/ipv4/tcp_output.c
- https://android.googlesource.com/kernel/common/+/refs/heads/android14-6.1/net/ipv4/tcp.c
