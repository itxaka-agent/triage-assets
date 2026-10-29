#!/bin/sh
set -e
W=/work
rm -rf $W/rootfs $W/keys $W/out; mkdir -p $W/rootfs/etc $W/rootfs/boot $W/keys $W/out
cat > $W/rootfs/etc/os-release <<OS
NAME="Kairos"
ID=kairos
KAIROS_RELEASE="v3.6.0"
KAIROS_FAMILY="ubuntu"
OS
# dummy kernel; build-uki only embeds it as a PE section
head -c 1048576 /dev/urandom > $W/rootfs/boot/vmlinuz-6.0
ln -s vmlinuz-6.0 $W/rootfs/boot/vmlinuz
cd $W/keys; for k in PK KEK db; do echo dummy > $k.auth; done
openssl req -new -x509 -newkey rsa:2048 -nodes -keyout db.key -out db.pem -days 1 -subj "/CN=test" 2>/dev/null
openssl genrsa -out tpm2-pcr-private.pem 2048 2>/dev/null
cat > $W/cc.yaml <<CC
#cloud-config
install:
  auto: true
  device: auto
users:
  - name: kairos
    passwd: MARKER_PASSWORD_9f3a
CC
/bin/auroraboot build-uki --output-dir $W/out -t iso --name test \
  --sb-key $W/keys/db.key --sb-cert $W/keys/db.pem --tpm-pcr-private-key $W/keys/tpm2-pcr-private.pem --public-keys $W/keys \
  --cloud-config $W/cc.yaml dir:$W/rootfs 2>&1 | tail -8
echo "=== ISO root listing ==="
xorriso -indev $W/out/test.iso -find / -type f 2>/dev/null
echo "=== grep marker in whole ISO (raw, and decompressing nothing) ==="
grep -c MARKER_PASSWORD_9f3a $W/out/test.iso || echo "marker NOT found in ISO bytes"
echo "=== extract efiboot.img and UKI, check initrd for marker ==="
mkdir -p $W/x; cd $W/x
xorriso -osirrox on -indev $W/out/test.iso -extract /efiboot.img efiboot.img >/dev/null 2>&1
mdir -/ -i efiboot.img ::/ 2>/dev/null | grep -i efi | head -20
mcopy -s -i efiboot.img ::/EFI ./EFI 2>/dev/null
find EFI -name '*.efi' | while read f; do
  objcopy -O binary --only-section=.initrd "$f" initrd.zst 2>/dev/null || continue
  [ -s initrd.zst ] || continue
  echo "-- $f"
  zstd -dq -f initrd.zst -o initrd.cpio 2>/dev/null || python3 - <<PY
PY
  if [ -f initrd.cpio ]; then
    echo "initrd entries under usr/local/cloud-config and oem:"; cpio -it < initrd.cpio 2>/dev/null | grep -E '^(usr/local/cloud-config|oem)' ; 
    grep -c MARKER_PASSWORD_9f3a initrd.cpio || echo "marker NOT in initrd"
    objcopy -O binary --only-section=.cmdline "$f" cmdline.txt 2>/dev/null; echo "cmdline: $(tr -d '\0' < cmdline.txt)"
  fi
  rm -f initrd.zst initrd.cpio
done
