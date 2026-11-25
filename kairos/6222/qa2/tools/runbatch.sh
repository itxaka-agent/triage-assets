#!/bin/bash
# usage: runbatch.sh batchfile watch ; each line: name|base|profile|conf|cmdline|pre|post
f=$1; w=$2
while IFS='|' read -r name base prof conf cmdl pre post; do
  [ "$pre" = FLIP ] && pre="$(cat /home/itxaka/projects/kairos-triage-agent/workspace/.scratch/qa6222b/tools/flip.sh)"
  python3 -I /home/itxaka/projects/kairos-triage-agent/workspace/.scratch/qa6222b/tools/scen.py "$name" "$base" "$([ "$prof" = - ] && echo a-unthrottled || echo $prof)" "$w" "$conf" "$cmdl" "$pre" "${post:--}" > /home/itxaka/projects/kairos-triage-agent/workspace/.scratch/qa6222b/logs/$name.out 2>&1
done < $f
