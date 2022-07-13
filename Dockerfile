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
        numactl \
        procps-ng \
        sysstat \
        dropwatch \
        wireshark-cli \
        https://rpmfind.net/linux/epel/8/Everything/x86_64/Packages/u/uperf-1.0.7-1.el8.x86_64.rpm \
    && dnf clean all \
    && rm -rf /var/cache/yum

COPY --from=builder /src/ovs-exporter /usr/bin
RUN chmod +x /usr/bin/ovs-exporter

CMD ["ovs-exporter"]
