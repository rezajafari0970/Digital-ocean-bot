/* Own-socket TCP_INFO reader. No packet capture, routing, or tuning. */
#include <sys/socket.h>
#include <netinet/in.h>
#include <linux/tcp.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>

int tcpdiag_snapshot(int fd, char *out, size_t cap) {
    struct tcp_info info;
    socklen_t len = sizeof(info);
    memset(&info, 0, sizeof(info));
    if (getsockopt(fd, IPPROTO_TCP, TCP_INFO, &info, &len) < 0)
        return -errno;
    size_t used = 0;
    int n;
#define ADD(...) do { \
    n = snprintf(out + used, cap > used ? cap - used : 0, __VA_ARGS__); \
    if (n < 0 || (size_t)n >= cap - used) return -ENOSPC; \
    used += (size_t)n; \
} while (0)
    ADD("{\"compiled_struct_bytes\":%zu,\"returned_bytes\":%u,\"fields\":{",
        sizeof(info), (unsigned)len);
#define FIELD(f) do { \
    ADD("\"" #f "\":"); \
    if ((size_t)len >= offsetof(struct tcp_info, f) + sizeof(info.f)) \
        ADD("%llu,", (unsigned long long)info.f); \
    else ADD("null,"); \
} while (0)
    FIELD(tcpi_state); FIELD(tcpi_ca_state); FIELD(tcpi_retransmits);
    FIELD(tcpi_rto); FIELD(tcpi_rtt); FIELD(tcpi_rttvar);
    FIELD(tcpi_snd_mss); FIELD(tcpi_rcv_mss);
    FIELD(tcpi_unacked); FIELD(tcpi_lost); FIELD(tcpi_total_retrans);
    FIELD(tcpi_bytes_acked); FIELD(tcpi_bytes_received);
    FIELD(tcpi_segs_out); FIELD(tcpi_segs_in);
    FIELD(tcpi_data_segs_out); FIELD(tcpi_data_segs_in);
    FIELD(tcpi_notsent_bytes); FIELD(tcpi_bytes_sent); FIELD(tcpi_bytes_retrans);
    FIELD(tcpi_rcv_ooopack); FIELD(tcpi_snd_wnd);
    out[--used] = '\0';
    ADD("}}");
    return (int)used;
#undef ADD
#undef FIELD
}
