#!/bin/bash

# Register serviceA
docker-compose exec spire-server /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://example.org/serviceA \
    -parentID spiffe://example.org/agent \
    -selector unix:uid:1001

# Register serviceB
docker-compose exec spire-server /opt/spire/bin/spire-server entry create \
    -spiffeID spiffe://example.org/serviceB \
    -parentID spiffe://example.org/agent \
    -selector unix:uid:1002

