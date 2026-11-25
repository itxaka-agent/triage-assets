import sys; sys.dont_write_bytecode=True; sys.path.insert(0,"/home/itxaka/projects/kairos-triage-agent/workspace/.scratch/qa6222b/tools")
from tq2 import *
LD=ROOT+"/logs/life"; os.makedirs(LD,exist_ok=True)
disk=ROOT+"/lanes/life.qcow2"
if os.path.exists(disk): os.unlink(disk)
subprocess.check_call(["qemu-img","create","-q","-f","qcow2","-F","raw","-b",ROOT+"/disks/d0.raw",disk])
out=open(LD+"/life.txt","w")
def P(*a):
    s=" ".join(str(x) for x in a); out.write(s+"\n"); out.flush()
CHK="cd /run/initramfs/cos-state/cOS; ls -la; for i in active passive; do echo $i; cat $i.img.sha256 | head -1; sha256sum $i.img; done; echo CONF; cat image-checksum.conf 2>&1; kairos-agent verify-images 2>&1 | tail -12; echo vi_rc=$?"
def boot(n,**k):
    v=VM(n,disk=disk,fmt="qcow2",logdir=LD,**k).start(); v.login(timeout=600); return v
def down(v):
    v.send("sync; poweroff")
    for _ in range(600):
        if not v.running(): break
        time.sleep(0.2)
    v.stop()
v=boot("life1")
try:
    P("== boot1 state"); P(v.sh("kairos-agent state | head -5",log=False)[1])
    v.sh("mount -o remount,rw /oem 2>&1; true",log=False)
    v.put("/oem/90_qa.yaml","#cloud-config\nimage-checksum:\n  create: true\n  verify: warn\n  verify-timeout: 30\n")
    rc,o=v.sh("kairos-agent upgrade --debug --allow-insecure-registries --source oci:10.0.2.2:55623/qa-up:v1 > /run/up.log 2>&1; echo up_rc=$?; grep -E 'Recorded sha256|sha256 of|policy' /run/up.log | cut -c1-200; tail -3 /run/up.log | cut -c1-200",timeout=1200)
    P("== upgrade"); P(o)
    P("== after upgrade"); P(v.sh(CHK,timeout=300)[1])
    v.sh("sync",log=False)
    v.send("reboot"); time.sleep(5); v.buf=""
    v.login(timeout=600)
    P("== boot after upgrade"); P(v.sh("ls /etc/qa6222-upgrade-marker; grep -E 'hecksum' /run/immucore/immucore.log | cut -c1-400",log=False)[1])
    v.sh("kairos-agent bootentry --select recovery; sync",log=False)
    v.send("reboot"); time.sleep(5); v.buf=""
    v.login(timeout=600)
    P("== recovery boot"); P(v.sh("grep -o 'cos-img/filename=[^ ]*' /proc/cmdline; grep -E 'hecksum' /run/immucore/immucore.log | cut -c1-400",log=False)[1])
    v.sh("mount -o remount,rw /oem 2>&1; true",log=False)
    rc,o=v.sh("kairos-agent reset --unattended --reboot=false --debug > /run/rs.log 2>&1; echo rs_rc=$?; grep -E 'Recorded sha256|sha256 of|policy' /run/rs.log | cut -c1-200",timeout=1200)
    P("== reset"); P(o)
    v.sh("mkdir -p /run/st; mount -o ro /dev/disk/by-label/COS_STATE /run/st 2>&1; true",log=False)
    P(v.sh(CHK.replace("/run/initramfs/cos-state","/run/st"),timeout=300)[1])
    v.sh("sync",log=False)
    v.send("reboot"); time.sleep(5); v.buf=""
    v.login(timeout=600)
    P("== boot after reset"); P(v.sh("grep -o 'cos-img/filename=[^ ]*' /proc/cmdline; grep -E 'hecksum' /run/immucore/immucore.log | cut -c1-400",log=False)[1])
except Exception as e:
    P("ERROR",repr(e)[:500])
finally:
    v.stop()
