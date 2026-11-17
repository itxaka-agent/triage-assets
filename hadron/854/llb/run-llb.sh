#!/bin/bash
S=/home/itxaka/projects/kairos-triage-agent/workspace/.scratch/hadron-854-qa; cd $S/llb; L=$S/bin/llbdump
H=/home/itxaka/projects/wt-kernel/Dockerfile; B=$S/wt-base/Dockerfile
C="-arg KERNEL_TYPE=default -arg FIPS=no-fips -arg SOURCE_DATE_EPOCH=0"
d(){ n=$1; f=$2; shift 2; $L -f $f "$@" $C > $n.txt 2>$n.err || { echo FAIL $n; tail -3 $n.err; }; }
d base-kernel-misc-novers $B -target kernel-misc -platform linux/amd64
d base-kernel-misc-main $B -target kernel-misc -platform linux/amd64 -arg VERSION=main
d head-kernel-misc-novers $H -target kernel-misc -platform linux/amd64
d head-kernel-misc-main $H -target kernel-misc -platform linux/amd64 -arg VERSION=main
d head-kernel-artifacts-novers $H -target kernel-artifacts -platform linux/amd64
d head-kernel-artifacts-main $H -target kernel-artifacts -platform linux/amd64 -arg VERSION=main
d base-image-grub $B -target default -platform linux/amd64 -arg VERSION=main -arg BOOTLOADER=grub
d head-image-grub $H -target default -platform linux/amd64 -arg VERSION=main -arg BOOTLOADER=grub
d head-image-systemd $H -target default -platform linux/amd64 -arg VERSION=main -arg BOOTLOADER=systemd
wc -l *.txt
R='^[[]kernel'
for p in base head; do ../bin/compare.sh . $p-kernel-misc-novers $p-kernel-misc-main "$R" | tail -3; done
../bin/compare.sh . head-kernel-artifacts-novers head-kernel-artifacts-main "$R" | tail -3
echo ALLVERTS; for p in base head; do ../bin/compare.sh . $p-kernel-misc-novers $p-kernel-misc-main '.' | tail -1; done
../bin/compare.sh . head-kernel-artifacts-novers head-kernel-artifacts-main '.' | tail -1
echo JOBVSIMAGE
for p in base head; do echo $p; ../bin/compare.sh . $p-kernel-misc-novers $p-image-grub "$R" | tail -3; done
../bin/compare.sh . head-kernel-artifacts-novers head-image-grub "$R" | tail -3
../bin/compare.sh . head-kernel-artifacts-novers head-image-systemd "$R" | tail -3
