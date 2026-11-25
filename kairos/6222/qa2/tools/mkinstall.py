import sys; sys.dont_write_bytecode=True; sys.path.insert(0,"/home/itxaka/projects/kairos-triage-agent/workspace/.scratch/qa6222b/tools")
from tq2 import *
# usage: mkinstall.py NAME SIZE_MIB|0 EXTRA_YAML_FILE_OR_-
name=sys.argv[1]; size=int(sys.argv[2]); extra=open(sys.argv[3]).read() if len(sys.argv)>3 and sys.argv[3]!="-" else ""
ISO=glob.glob(ROOT+"/kairos/build/iso/*.iso")[0]
ci=mkcidata(ROOT+"/cfg/login.iso", "#cloud-config\nstages:\n  initramfs:\n    - users:\n        kairos:\n          passwd: %s\n          groups: [admin]\n"%TEST_PW)
disk=ROOT+"/disks/%s.raw"%name
if os.path.exists(disk): os.unlink(disk)
subprocess.check_call(["qemu-img","create","-f","raw",disk,"20G" if size==0 else "32G"],stdout=subprocess.DEVNULL)
src="" if size==0 else "--source oci:10.0.2.2:55623/qa-big:v1 --allow-insecure-registries"
sz="" if size==0 else "  system:\n    size: %d\n  recovery-system:\n    size: %d\n  passive:\n    size: %d\n"%(size,size,size)
cfg="#cloud-config\ninstall:\n  device: /dev/vda\n  reboot: false\n"+sz+extra+"users:\n  - name: kairos\n    passwd: %s\n    groups: [admin]\n"%TEST_PW
v=VM("ins-"+name,disk=disk,iso=ISO,cidata=ci,mem=4096,logdir=ROOT+"/logs").start()
try:
    v.login()
    v.put("/run/cfg.yaml",cfg)
    rc,out=v.sh("kairos-agent version; kairos-agent manual-install --debug %s --device /dev/vda /run/cfg.yaml > /run/inst.log 2>&1; echo rc=$?; grep -i -E 'sha256|checksum|policy' /run/inst.log | cut -c1-240; mkdir -p /run/st /run/rc; mount -o ro /dev/disk/by-label/COS_STATE /run/st; mount -o ro /dev/disk/by-label/COS_RECOVERY /run/rc; ls -la /run/st/cOS /run/rc/cOS; for f in /run/st/cOS/image-checksum.conf /run/rc/cOS/image-checksum.conf; do echo == $f; cat $f; done; cd /run/st/cOS && sha256sum active.img passive.img; cat active.img.sha256 passive.img.sha256; cd /run/rc/cOS; sha256sum recovery.img; cat recovery.img.sha256"%src,timeout=3600)
    print(out)
    open(ROOT+"/logs/ins-%s.check.txt"%name,"w").write(out)
    v.sh("sync; umount /run/st /run/rc; poweroff",timeout=60)
    time.sleep(15)
finally:
    v.stop()
