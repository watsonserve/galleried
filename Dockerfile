FROM golang AS builder
WORKDIR /home
# RUN apt-get update && apt-get -y install libopencv-dev libheif-dev
RUN git clone https://github.com/watsonserve/galleried.git && \
cd ./galleried && export GOPROXY=goproxy.cn && go build -ldflags "-s -w" -buildvcs=false

FROM ubuntu:24.04
# RUN apt-get update && apt-get -y install libopencv-core406t64 libopencv-imgproc406t64 libopencv-imgcodecs406t64 && rm -rf /var/lib/apt/lists/*
COPY --from=builder /home/galleried/galleried /usr/local/bin/
