"""Read TCP_INFO for sockets owned by this process; never a wire capture."""
import argparse
import ctypes
import datetime
import hashlib
import json
import os
from pathlib import Path
import queue
import secrets
import socket
import threading
import time


class TcpInfo:
    def __init__(self, library):
        self.library = str(Path(library).resolve(strict=True))
        self.lib = ctypes.CDLL(self.library)
        self.lib.tcpdiag_snapshot.argtypes = [ctypes.c_int, ctypes.c_void_p,
                                             ctypes.c_size_t]
        self.lib.tcpdiag_snapshot.restype = ctypes.c_int

    def snapshot(self, sock, label):
        buf = ctypes.create_string_buffer(4096)
        n = self.lib.tcpdiag_snapshot(sock.fileno(), buf, len(buf))
        if n < 0:
            raise OSError(-n, os.strerror(-n))
        out = json.loads(buf.value)
        out.update(label=label, monotonic_ns=time.monotonic_ns(),
                   utc=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                   local=sock.getsockname(), peer=sock.getpeername(),
                   scope="own_socket_kernel_counters_not_wire_capture")
        return out


def read_exact(sock, count):
    data = bytearray()
    while len(data) < count:
        part = sock.recv(count - len(data))
        if not part:
            raise EOFError(f"received {len(data)} of {count}")
        data.extend(part)
    return bytes(data)


def selftest(reader, up, down):
    # Loopback only. This checks instrumentation, never carrier reachability.
    nonce = secrets.token_hex(16).encode()
    upload = secrets.token_bytes(up)
    expected = nonce + b"\n" + hashlib.sha256(upload).hexdigest().encode() + b"\n" + b"R" * down
    results = queue.Queue()
    listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    listener.settimeout(3)
    listener.bind(("127.0.0.1", 0))
    listener.listen(1)

    def server():
        try:
            with listener:
                conn, _ = listener.accept()
                with conn:
                    conn.settimeout(3)
                    start = reader.snapshot(conn, "server_accepted")
                    received = read_exact(conn, up)
                    conn.sendall(nonce + b"\n" + hashlib.sha256(received).hexdigest().encode()
                                 + b"\n" + b"R" * down)
                    time.sleep(0.2)
                    end = reader.snapshot(conn, "server_after_transfer")
                    results.put({"upload_sha_ok": received == upload,
                                 "snapshots": [start, end]})
        except Exception as exc:
            results.put({"error": repr(exc)})

    th = threading.Thread(target=server, daemon=True)
    th.start()
    with socket.create_connection(listener.getsockname(), timeout=3) as client:
        snapshots = [reader.snapshot(client, "client_connected")]
        client.sendall(upload)
        snapshots.append(reader.snapshot(client, "client_upload_written"))
        received = read_exact(client, len(expected))
        snapshots.append(reader.snapshot(client, "client_response_read"))
        time.sleep(0.1)
        snapshots.append(reader.snapshot(client, "client_settled"))
    th.join(4)
    other = results.get(timeout=1)
    f = snapshots[-1]["fields"]
    passed = (received == expected and other.get("upload_sha_ok") is True
              and f["tcpi_bytes_received"] is not None
              and f["tcpi_bytes_received"] >= len(expected)
              and f["tcpi_bytes_acked"] is not None
              and f["tcpi_bytes_acked"] >= up
              and f["tcpi_segs_out"] is not None
              and f["tcpi_segs_out"] > 0)
    return {"scope": "loopback_instrumentation_selftest_only",
            "upload": up, "response_bytes": len(received),
            "body_sha256": hashlib.sha256(received).hexdigest(),
            "passed": passed, "client": snapshots, "server": other}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--library", required=True)
    parser.add_argument("--self-test", action="store_true", required=True)
    args = parser.parse_args()
    reader = TcpInfo(args.library)
    tests = [selftest(reader, up, down) for up, down in
             [(128, 128), (128, 65536), (16384, 128), (16384, 65536)]]
    # Invalid descriptors must fail; a missing field must never become zero.
    invalid_fd_rejected = False
    with socket.socket() as closed:
        pass
    try:
        reader.snapshot(closed, "invalid")
    except OSError as exc:
        invalid_fd_rejected = exc.errno == 9
    result = {"schema": 1, "scope": "instrumentation_only_not_target_test",
              "uid": os.getuid(), "uname": list(os.uname()),
              "library_sha256": hashlib.sha256(Path(reader.library).read_bytes()).hexdigest(),
              "invalid_fd_rejected": invalid_fd_rejected, "tests": tests,
              "passed": all(t["passed"] for t in tests) and invalid_fd_rejected}
    print(json.dumps(result))
    return 0 if result["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
