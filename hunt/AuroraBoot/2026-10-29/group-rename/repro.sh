#!/bin/bash
set -e
apt-get update -qq >/dev/null && apt-get install -y -qq libpcsclite-dev curl jq >/dev/null 2>&1
cd /src && go build -o /tmp/auroraboot . 2>&1 | tail -5
mkdir -p /tmp/data
/tmp/auroraboot web --listen 127.0.0.1:8080 --data-dir /tmp/data --admin-password adminpw --registration-token regtok --url http://127.0.0.1:8080 --disable-rate-limit >/tmp/server.log 2>&1 &
for i in $(seq 1 60); do curl -sf http://127.0.0.1:8080/healthz >/dev/null && break; sleep 1; done
A="Authorization: Bearer adminpw"
echo "== create group 'edge-prod'"
G=$(curl -s -H "$A" -H 'Content-Type: application/json' -d '{"name":"edge-prod"}' http://127.0.0.1:8080/api/v1/groups)
echo "$G"; GID=$(echo "$G" | jq -r .id)
echo "== create artifact targeting group $GID"
ART=$(curl -s -H "$A" -H 'Content-Type: application/json' -d "{\"name\":\"img\",\"baseImage\":\"quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.5.0\",\"arch\":\"amd64\",\"outputs\":{\"iso\":true},\"provisioning\":{\"autoInstall\":true,\"registerAuroraBoot\":true,\"targetGroupId\":\"$GID\"}}" http://127.0.0.1:8080/api/v1/artifacts)
AID=$(echo "$ART" | jq -r .id); echo "artifact id=$AID"
echo "== baked cloud-config phonehome block"
curl -s -H "$A" http://127.0.0.1:8080/api/v1/artifacts/$AID | jq -r .cloudConfig | sed -n '/^phonehome:/,/^[a-z]/p'
echo "== rename group to 'edge-production'"
curl -s -X PUT -H "$A" -H 'Content-Type: application/json' -d '{"name":"edge-production"}' http://127.0.0.1:8080/api/v1/groups/$GID; echo
echo "== artifact record after rename (targetGroupId, cloudConfig group)"
curl -s -H "$A" http://127.0.0.1:8080/api/v1/artifacts/$AID | jq -c '{targetGroupId, group: (.cloudConfig|capture("group: (?<g>[^\n]*)").g)}'
echo "== node from that image registers (agent sends phonehome.group='edge-prod')"
R=$(curl -s -w ' HTTP%{http_code}' -H 'Content-Type: application/json' -d '{"registrationToken":"regtok","machineID":"m-old-image","hostname":"n1","group":"edge-prod"}' http://127.0.0.1:8080/api/v1/nodes/register); echo "$R"
N1=$(echo "$R" | sed 's/ HTTP.*//' | jq -r .id)
echo "== control: node sending new name registers"
R2=$(curl -s -w ' HTTP%{http_code}' -H 'Content-Type: application/json' -d '{"registrationToken":"regtok","machineID":"m-new-name","hostname":"n2","group":"edge-production"}' http://127.0.0.1:8080/api/v1/nodes/register); echo "$R2"
N2=$(echo "$R2" | sed 's/ HTTP.*//' | jq -r .id)
echo "== node groups"
curl -s -H "$A" http://127.0.0.1:8080/api/v1/nodes | jq -c '.[] | {hostname, groupID}'
echo "== members of group $GID"
curl -s -H "$A" "http://127.0.0.1:8080/api/v1/nodes?group=$GID" | jq -c '[.[].hostname]'
echo "== server log lines mentioning group/warn"
grep -i -E "group|warn" /tmp/server.log | grep -v "GET\|POST\|PUT" | head || true
kill %1 || true
