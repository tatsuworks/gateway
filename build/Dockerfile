FROM golang:1.13.3

ENV FDB_URL "https://www.foundationdb.org/downloads/6.2.7/ubuntu/installers/foundationdb-clients_6.2.7-1_amd64.deb"

RUN apt update && apt install -y wget zlib1g zlib1g-dev

RUN wget -O fdb.deb $FDB_URL &&  dpkg -i fdb.deb

COPY build.sh /
RUN chown 1000:1000 /build.sh
