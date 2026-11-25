import json,sys
seen={}
for l in open("/home/itxaka/projects/kairos-triage-agent/workspace/.scratch/qa6222b/logs/results.jsonl"):
    r=json.loads(l); seen[r["name"]]=r
for n,r in seen.items():
    if len(sys.argv)>1 and not n.startswith(tuple(sys.argv[1:])): continue
    print(n,"|",r.get("outcome"),"login",r.get("login_s"),"up",r.get("uptime"),"post_rc",r.get("post_rc"),"reboots",r.get("reboots"),"oot",r.get("ran_out_of_time_on_serial"))
    for x in r.get("lines",[]): print("    ",x[:420])
