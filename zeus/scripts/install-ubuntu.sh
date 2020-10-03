#!/bin/bash -e

## Clone and compile DPI dependencies from source

apt update
apt install -y libpcap-dev software-properties-common ca-certificates liblzo2-2 libkeyutils-dev
update-ca-certificates
apt-get install -y apt-transport-https curl lsb-release wget autogen autoconf libtool gcc libpcap-dev linux-headers-generic git vim autoconf automake libtool make g++ bison flex cmake binutils binutils-doc gcc-doc cmake-doc extra-cmake-modules

wget https://github.com/wanduow/wandio/archive/4.2.2-1.tar.gz
tar xfz 4.2.2-1.tar.gz
cd wandio-4.2.2-1 && ./bootstrap.sh && ./configure && make && make install
cd ..

wget https://github.com/LibtraceTeam/libtrace/archive/4.0.11-1.tar.gz
tar xfz 4.0.11-1.tar.gz
cd libtrace-4.0.11-1 && ./bootstrap.sh && ./configure && make && make install
cd ..

wget https://github.com/wanduow/libflowmanager/archive/3.0.0.tar.gz
tar xfz 3.0.0.tar.gz
cd libflowmanager-3.0.0 && ./bootstrap.sh && ./configure && make && make install
cd ..

wget https://github.com/wanduow/libprotoident/archive/2.0.14-1.tar.gz
tar xfz 2.0.14-1.tar.gz
cd libprotoident-2.0.14-1 && ./bootstrap.sh && ./configure && make && make install
cd ..

wget https://github.com/ntop/nDPI/archive/3.2.tar.gz
tar xfz 3.2.tar.gz
cd nDPI-3.2 && ./autogen.sh && ./configure && make && make install
cd ..

apt install -y liblinear-dev

go mod download

export CFLAGS="-I/usr/local/lib"
export CPPFLAGS="-I/usr/local/lib"
export CXXFLAGS="-I/usr/local/lib"
export LDFLAGS="--verbose -v -L/usr/local/lib -llinear -ltrace -lndpi -lpcap -lm -pthread"
export LD_LIBRARY_PATH="/usr/local/lib:/usr/lib:/go"
export LD_RUN_PATH="/usr/local/lib"

ldconfig /usr/local/lib/*
ldconfig /go/*

env
#find / -iname ndpi_main.h
#find / -iname libprotoident.h
#find / -iname libtrace.h

go build -mod=readonly -ldflags "-s -w -X github.com/dreadl0ck/netcap.Version=v${VERSION}" -o /usr/local/bin/net -i github.com/dreadl0ck/netcap/cmd

