QA of kairos-io/kairos#6226 head 0ae646cfc, ISO built with KUBERNETES_DISTRO= make iso; 4 QEMU/KVM VMs (A seeder, B downloader, C 2nd seeder + rogue proxy at 10.77.0.4, D fresh install) on one QEMU mcast L2 segment 10.77.0.0/24.
Upgrade proof: A passive.img got a marker file /etc/qa-marker (QA6226-MARKER-1795625413), sha256 df07836f...; B upgraded from p2p://10.77.0.1/passive?pin=b94bc49f..., downloaded sha256 df07836f (equal), rebooted active_boot, cat /etc/qa-marker printed the marker.
Install proof: D manual-install --source p2p://10.77.0.1,10.77.0.3/active?pin=d3329f3a.. (2 identical seeders, 389+379 chunks), downloaded sha 780132ec (= A active), D booted.
Reset proof: B recovery reset with reset.system.source p2p (2 peers), see reset-B-agent-log.txt.
Wrong pin / no pin / sha256 / insecure / hostile peers: see mentioned logs in this dir and report.
