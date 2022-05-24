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
    && dnf clean all \
    && rm -rf /var/cache/yum

CMD ["sleep", "infinity"]
