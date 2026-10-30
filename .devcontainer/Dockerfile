FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9

RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends\
        gettext-base \
        iptables \
        libgl1-mesa-dev \
        xorg-dev \
        libayatana-appindicator3-dev \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && go install -v golang.org/x/tools/gopls@latest


WORKDIR /app
