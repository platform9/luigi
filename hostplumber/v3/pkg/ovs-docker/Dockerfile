# openvswitch-2.12.0-r3:
# A production quality, multilayer virtual switch

FROM alpine:3.12

RUN apk add --no-cache bash
RUN apk add --no-cache openvswitch
RUN apk add --no-cache supervisor

RUN mkdir -p /var/log/supervisor

COPY supervisord.conf /etc/
COPY ovs.conf /etc/supervisor/conf.d/

CMD ["/usr/bin/supervisord"]

