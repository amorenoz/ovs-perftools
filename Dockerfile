FROM  quay.io/centos/centos:stream8 as builder

RUN dnf install --best --refresh -y \
    golang \
    && dnf clean all \
    && rm -rf /var/cache/yum

ADD . /src

WORKDIR /src
RUN go build cmd/ovs-exporter.go

# ---- #
FROM quay.io/centos/centos:stream8

USER root

RUN dnf install -y centos-release-nfv-openvswitch
RUN dnf install --best --refresh -y \
        openvswitch2.15 \
        perf \
        bcc \
        bpftrace \
        bpftool \
        numactl \
        procps-ng \
        sysstat \
        ethtool \
        dropwatch \
        tcpdump \
        wireshark-cli \
        strace \
        python3-pip \
        https://rpmfind.net/linux/epel/8/Everything/x86_64/Packages/u/uperf-1.0.7-1.el8.x86_64.rpm \
    && dnf clean all \
    && rm -rf /var/cache/yum

ADD bpftrace/* /usr/share/bpftrace/tools/

RUN python3 -m pip install ovs-dbg scapy

COPY --from=builder /src/ovs-exporter /usr/bin
RUN chmod +x /usr/bin/ovs-exporter

#CMD ["ovs-exporter"]
CMD ["sleep", "infinity"]
