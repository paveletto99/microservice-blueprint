FROM golang:1.22-bookworm@sha256:5c56bd47228dd572d8a82971cf1f946cd8bb1862a8ec6dc9f3d387cc94136976

RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends\
        gettext-base=0.21-4 \
        iptables=1.8.7-1 \
        libgl1-mesa-dev=20.3.5-1 \
        xorg-dev=1:7.7+22 \
        libayatana-appindicator3-dev=0.5.5-2+deb11u2 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && go install -v golang.org/x/tools/gopls@latest


WORKDIR /app