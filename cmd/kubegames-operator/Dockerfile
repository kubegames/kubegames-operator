FROM busybox:latest

ADD Shanghai /etc/localtime
RUN echo 'Asia/Shanghai' >/etc/timezone

ADD kubegames-operator /kubegames-operator
RUN chmod -R 777 /kubegames-operator

WORKDIR /

ENTRYPOINT ./kubegames-operator