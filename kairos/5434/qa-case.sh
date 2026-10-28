#!/bin/sh
# guest; $1 = before|head
r=$1; B=/tmp/agent-$r; d=/tmp/in; mkdir -p $d
printf '\n#cloud-config\nhostname: a\n' > $d/1-blank-first.yaml
printf '# My cluster config\n#cloud-config\nhostname: a\n' > $d/2-comment-first.yaml
printf '#cloud-config \nhostname: a\n' > $d/3-trailing-space.yaml
printf '#cloud-config\nhostname: a\n' > $d/4-plain.yaml
printf '#cloud-configuration\nhostname: a\n' > $d/5-cloud-configuration.yaml
{ for i in 1 2 3 4 5 6 7 8; do echo "# c$i"; done; echo '#cloud-config'; echo 'hostname: a'; } > $d/6-header-line9.yaml
{ for i in 1 2 3 4 5 6 7 8 9; do echo "# c$i"; done; echo '#cloud-config'; echo 'hostname: a'; } > $d/7-header-line10.yaml
{ for i in 1 2 3 4 5 6 7 8 9 10; do echo "# c$i"; done; echo '#cloud-config'; echo 'hostname: a'; } > $d/8-header-line11.yaml
printf 'hostname: a\n' > $d/9-no-header.yaml
echo "##### BINARY: $r ($B)"
for f in $d/*.yaml; do printf '%-30s ' $(basename $f); $B validate $f 2>&1 | tr '\n' ' '; echo "[rc=$?]"; done
