import importlib.util
import json
import os
import pathlib
import socket
import struct
import sys
import threading
import types

spec = importlib.util.spec_from_file_location("probe", pathlib.Path(__file__).with_name("probe.py"))
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)


def dns_reply(request):
    return request[:2] + b"\x81\x80\x00\x01\x00\x01\x00\x00\x00\x00" + request[12:] + b"\xc0\x0c\x00\x01\x00\x01\x00\x00\x00\x3c\x00\x04\x01\x01\x01\x01"


def quic_reply(request):
    return b"\x80" + bytes(4) + b"\x08" + request[15:23] + b"\x08" + request[6:14] + b"\x00\x00\x00\x01"


def validation():
    dns = probe.dns_request()
    quic = probe.quic_request()
    assert probe.dns_valid(dns_reply(dns), dns)
    assert probe.quic_valid(quic_reply(quic), quic)
    for wrong in (dns, dns[:2], b"xx" + dns_reply(dns)[2:],
                  dns[:2] + b"\x81\x83" + dns_reply(dns)[4:]):
        assert not probe.dns_valid(wrong, dns)
    for wrong in (quic, bytes(40), quic_reply(probe.quic_request()),
                  quic_reply(quic)[:-1], quic_reply(quic)[:23] + quic[1:5]):
        assert not probe.quic_valid(wrong, quic)
    header = b"\0\0\0\x01" + socket.inet_aton("1.1.1.1") + struct.pack("!H", 443)
    assert probe.datagram_payload(header + quic, "1.1.1.1", 443) == quic
    assert probe.datagram_payload(header + quic, "1.0.0.1", 443) is None
    assert probe.datagram_payload(b"\0\0\1" + header[3:] + quic, "1.1.1.1", 443) is None
    assert probe.datagram_payload(header + quic, "1.1.1.1", 53) is None


class Socks:
    def __init__(self, allow, forged=False):
        self.allow, self.forged = allow, forged
        self.tcp = socket.socket()
        self.tcp.bind(("127.0.0.1", 0))
        self.tcp.listen()
        self.udp = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        self.udp.bind(("127.0.0.1", 0))
        threading.Thread(target=self.accept, daemon=True).start()
        threading.Thread(target=self.datagrams, daemon=True).start()

    def accept(self):
        while True:
            try:
                client, _ = self.tcp.accept()
            except OSError:
                return
            threading.Thread(target=self.connection, args=(client,), daemon=True).start()

    def connection(self, client):
        try:
            with client:
                client.settimeout(10)
                greeting = probe.read_exact(client, 2)
                probe.read_exact(client, greeting[1])
                client.sendall(b"\x05\x00")
                header = probe.read_exact(client, 4)
                if header[3] == 1:
                    probe.read_exact(client, 6)
                elif header[3] == 3:
                    probe.read_exact(client, probe.read_exact(client, 1)[0] + 2)
                else:
                    probe.read_exact(client, 18)
                if header[1] != 3 or not self.allow:
                    client.sendall(b"\x05\x09\x00\x01" + bytes(6))
                    return
                client.sendall(b"\x05\x00\x00\x01" + socket.inet_aton("127.0.0.1") + struct.pack("!H", self.udp.getsockname()[1]))
                while client.recv(64):
                    pass
        except (OSError, ValueError):
            pass

    def datagrams(self):
        while True:
            try:
                data, origin = self.udp.recvfrom(4096)
                if data[:4] != b"\0\0\0\x01" or len(data) < 12:
                    continue
                payload = data[10:]
                port = struct.unpack("!H", data[8:10])[0]
                answer = dns_reply(payload) if port == 53 else quic_reply(payload)
                if self.forged:
                    answer = bytes(len(answer))
                self.udp.sendto(data[:10] + answer, origin)
            except OSError:
                return

    def candidate(self, label):
        outbound = {"protocol": "socks", "settings": {"servers": [
            {"address": "127.0.0.1", "port": self.tcp.getsockname()[1]}]}}
        return {"id": label, "driver": "socks5", "digest": "a" * 64, "outbound": outbound}

    def close(self):
        self.tcp.close()
        self.udp.close()


def native():
    core = pathlib.Path(os.environ["XRAY_TEST_BINARY"])
    digest = probe.sha_file(core)
    # The production entrypoint has no path override. The fixture substitutes
    # only the pinned binary path; all SOCKS/native/UDP work runs unmodified.
    probe.pathlib = types.SimpleNamespace(Path=lambda _: core)
    fixtures = [Socks(True), Socks(False), Socks(True, forged=True)]
    try:
        result = probe.probe({"core_hashes": [digest], "candidates": [
            fixture.candidate(str(i)) for i, fixture in enumerate(fixtures)]})
        assert [r["code"] for r in result["results"]] == [
            "ok", "datagram_unverified", "datagram_unverified"], result
        assert result["results"][0]["dns"] and result["results"][0]["quic"]
        for row in result["results"][1:]:
            assert not row["dns"] and not row["quic"]
        try:
            probe.probe({"core_hashes": ["unknown"], "candidates": [fixtures[0].candidate("one")]})
        except ValueError:
            pass
        else:
            raise AssertionError("unknown core admitted")
    finally:
        for fixture in fixtures:
            fixture.close()


validation()
if sys.argv[1] == "native":
    native()
print("protocol probe contracts passed")
