#!/bin/bash
set -u
apt-get update -qq >/dev/null && apt-get install -y -qq libpcsclite-dev sqlite3 curl >/dev/null 2>&1
cp -r /src /build && cd /build
mkdir -p ui/dist 2>/dev/null; true
go build -o /tmp/ab . 2>&1 | tail -20 || exit 1
mkdir -p /tmp/d
/tmp/ab web --listen 127.0.0.1:8099 --data-dir /tmp/d --admin-password adminpw --registration-token regtok >/tmp/srv.log 2>&1 &
for i in $(seq 1 60); do curl -s -o /dev/null http://127.0.0.1:8099/ && break; sleep 1; done
A='Authorization: Bearer adminpw'
U=http://127.0.0.1:8099/api/v1
echo "== register node"
R=$(curl -s -X POST $U/nodes/register -H 'Content-Type: application/json' -d '{"registrationToken":"regtok","machineID":"m1","hostname":"h1"}'); echo "$R"
NID=$(echo "$R" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
echo "== list groups"; curl -s -H "$A" $U/groups; echo
echo "== PUT existing node -> nonexistent group"
curl -s -w ' HTTP %{http_code}\n' -X PUT -H "$A" -H 'Content-Type: application/json' $U/nodes/$NID/group -d '{"groupID":"no-such-group-0000"}'
echo "== GET node after"
curl -s -H "$A" $U/nodes/$NID; echo
echo "== DB row"; sqlite3 /tmp/d/auroraboot.db "select id, group_id from managed_nodes;"
echo "== DB node_groups count"; sqlite3 /tmp/d/auroraboot.db "select count(*) from node_groups;"
echo "== PRAGMA foreign_keys"; sqlite3 /tmp/d/auroraboot.db "pragma foreign_keys;"
echo "== create real group"
G=$(curl -s -X POST -H "$A" -H 'Content-Type: application/json' $U/groups -d '{"name":"g1"}'); echo "$G"
GID=$(echo "$G" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
echo "== PUT nonexistent node -> real group"
curl -s -w ' HTTP %{http_code}\n' -X PUT -H "$A" -H 'Content-Type: application/json' $U/nodes/no-such-node/group -d "{\"groupID\":\"$GID\"}"
echo "== PUT nonexistent node -> null group"
curl -s -w ' HTTP %{http_code}\n' -X PUT -H "$A" -H 'Content-Type: application/json' $U/nodes/no-such-node/group -d '{"groupID":null}'
echo "== GET nonexistent node (baseline)"
curl -s -w ' HTTP %{http_code}\n' -H "$A" $U/nodes/no-such-node
echo "== PUT labels nonexistent node"
curl -s -w ' HTTP %{http_code}\n' -X PUT -H "$A" -H 'Content-Type: application/json' $U/nodes/no-such-node/labels -d '{"labels":{"a":"b"}}'
echo "== list nodes in group g1 / ListByGroup"
curl -s -H "$A" "$U/nodes?group=no-such-group-0000"; echo
echo "== GET nonexistent group"
curl -s -w ' HTTP %{http_code}\n' -H "$A" $U/groups/no-such-group-0000
kill %1 2>/dev/null
echo "== server log tail"; tail -5 /tmp/srv.log
