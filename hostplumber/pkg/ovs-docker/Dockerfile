FROM alpine:3.18

RUN apk update && apk upgrade
RUN apk add --no-cache bash
RUN apk add --no-cache supervisor

RUN mkdir -p /var/log/supervisor

COPY supervisord.conf /etc/
COPY ovs.conf /etc/supervisor/conf.d/

RUN apk add --no-cache make gcc linux-headers bsd-compat-headers binutils coreutils diffutils && \
    apk add --no-cache gettext bash grep sed texinfo perl util-linux vim build-base libexecinfo libexecinfo-dev && \
    apk add --no-cache abuild bc cmake ncurses-dev ca-certificates wget && \
    apk add numactl-dev alpine-sdk bsd-compat-headers libexecinfo-dev py3-pip py3-elftools && \
    pip3 install meson ninja && \
    wget http://dpdk.org/browse/dpdk-stable/snapshot/dpdk-stable-23.11.2.zip && \
    unzip dpdk-stable-23.11.2.zip 

WORKDIR dpdk-stable-23.11.2
RUN meson build && \
    ninja -C build && \
    ninja -C build install

WORKDIR /
RUN apk add --no-cache autoconf automake libtool && \
    wget https://www.openvswitch.org/releases/openvswitch-3.3.2.tar.gz && \
    gunzip openvswitch-3.3.2.tar.gz && \
    tar -xf openvswitch-3.3.2.tar 

WORKDIR openvswitch-3.3.2 
RUN ./configure  --localstatedir=/var --sysconfdir=/etc --with-dpdk=static  && \
    make && \
    make install

WORKDIR /
RUN apk add pciutils
ADD start-ovs.sh /usr/local/bin/start-ovs.sh
RUN echo dpdk-init=false > /etc/openvswitch/ovs.conf
RUN chmod 777 /usr/local/bin/start-ovs.sh

CMD if $EnableDpdk == true ; then echo dpdk-init=true \
    $'\n'dpdk-hugepage-dir=/dev/hugepages$'\n'dpdk-socket-mem="$SocketMem" \
    $'\n'dpdk-lcore-mask="$LcoreMask"$'\n'pmd-cpu-mask="$PmdCpuMask"$'\n'  > /etc/openvswitch/ovs.conf ; fi && \
    /usr/bin/supervisord
