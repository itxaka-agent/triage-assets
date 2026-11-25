import sys; sys.dont_write_bytecode=True; sys.path.insert(0,"/home/itxaka/projects/kairos-triage-agent/workspace/.scratch/qa6222b/tools")
from tq2 import *
# usage: scen.py NAME BASE PROFILE WATCH_S CONF CMDLINE [PRE_SHELL]
#  CONF: literal conf file content with \n escapes, or "-" for none. CMDLINE: extra_active_cmdline or "-".
name,base,profile,watch=sys.argv[1:5]; watch=int(watch)
conf=sys.argv[5].encode().decode("unicode_escape") if sys.argv[5]!="-" else None
cmdl=sys.argv[6] if sys.argv[6]!="-" else None
pre=sys.argv[7].strip() if len(sys.argv)>7 and sys.argv[7]!="-" else None
post=sys.argv[8] if len(sys.argv)>8 and sys.argv[8]!="-" else None
LD=ROOT+"/logs/"+name; os.makedirs(LD,exist_ok=True)
disk=ROOT+"/lanes/%s.qcow2"%name; os.makedirs(ROOT+"/lanes",exist_ok=True)
if os.path.exists(disk): os.unlink(disk)
subprocess.check_call(["qemu-img","create","-q","-f","qcow2","-F","raw","-b",ROOT+"/disks/%s.raw"%base,disk])
rec=dict(name=name,base=base,profile=profile,conf=conf,cmdline=cmdl)
# config boot, unthrottled
if conf is not None or cmdl is not None or pre:
    v=VM(name+"-cfg",disk=disk,fmt="qcow2",logdir=LD).start()
    try:
        v.login(timeout=600)
        v.sh("mount -o remount,rw /run/initramfs/cos-state",log=True)
        if conf is not None:
            v.put("/run/conf.txt",conf)
            v.sh("cp /run/conf.txt /run/initramfs/cos-state/cOS/image-checksum.conf; cat /run/initramfs/cos-state/cOS/image-checksum.conf")
        if pre: v.sh(pre,timeout=600)
        if cmdl is not None:
            v.sh("kairos-agent grubedit set extra_active_cmdline='%s' 2>&1 | tail -3; cat /oem/grubenv 2>/dev/null | grep -a extra"%cmdl)
        v.sh("sync; mount -o remount,ro /run/initramfs/cos-state; sync")
        v.send("poweroff")
        for _ in range(600):
            if not v.running(): break
            time.sleep(0.2)
    finally: v.stop()
v=VM(name,disk=disk,profile=profile,fmt="qcow2",logdir=LD).start()
t_end=time.monotonic()+watch; login=None
try:
    while time.monotonic()<t_end:
        with v.lock:
            if re.search(r"login: ?$|automatic login",v.buf,re.M): login=time.monotonic()-v.t0; break
            fail=re.search(r"BOOT FAILED",v.buf)
        if fail:
            rec["failscreen_s"]=round(time.monotonic()-v.t0,1); time.sleep(4); v.shot(LD+"/failure-screen.png"); break
        time.sleep(1)
    rec["login_s"]=round(login,1) if login else None
    if login:
        v.login(timeout=60)
        rec["cmdline_seen"]=v.sh("cat /proc/cmdline",log=False)[1]
        rec["uptime"]=v.sh("cut -d' ' -f1 /proc/uptime",log=False)[1].strip()
        il=v.sh("cat /run/immucore/immucore.log",log=False,timeout=120)[1]
        open(LD+"/immucore.log","w").write(il)
        rec["lines"]=[l[:600] for l in il.splitlines() if re.search(r"hecksum|sha256|verify",l)]
        if post:
            rc,o=v.sh(post,timeout=900,log=False); open(LD+"/post.txt","w").write("rc=%d\n%s\n"%(rc,o)); rec["post_rc"]=rc
        open(LD+"/state.txt","w").write(v.sh("kairos-agent state 2>&1 | head -20; ls -la /run/initramfs/cos-state/cOS 2>&1",log=False)[1])
    v.shot(LD+"/screen-final.png")
    time.sleep(0.2)
finally:
    time.sleep(1)
    v.stop()
t=open(v.serial_log,errors="replace").read()
rec["reboots"]=len(re.findall(r"reboot: Restarting system",t))
rec["ran_out_of_time_on_serial"]="ran out of time" in t
rec["outcome"]="booted" if login else ("FAILURE SCREEN at %ss"%rec["failscreen_s"] if rec.get("failscreen_s") else "BOOT LOOP (%d reboots)"%rec["reboots"] if rec["reboots"] else "no login in %ds"%watch)
record(ROOT+"/logs/results.jsonl",**rec)
print(json.dumps(rec,indent=1),flush=True)
os.unlink(disk)
