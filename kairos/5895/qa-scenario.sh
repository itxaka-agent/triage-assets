set -u
mkdir -p /oem/sub/dir /usr/local/cloud-config/deep
printf '#cloud-config\nphonehome:\n  url: https://example.invalid\n  registration_token: <redacted>\nstack:\n  keep: me\n' > /oem/a.yaml
printf '#cloud-config\nphonehome:\n  url: https://example.invalid\nusers:\n- name: keepuser\n' > /oem/sub/dir/b.yml
printf '#cloud-config\nphonehome:\n  url: https://example.invalid\nhostname: keephost\n' > /usr/local/cloud-config/c.yaml
printf '#cloud-config\nphonehome:\n  url: https://example.invalid\nstages:\n  boot:\n  - name: x\n' > /usr/local/cloud-config/deep/d.yaml
printf 'phonehome:\n  url: x\n' > /oem/sub/note.txt
dump(){ for f in /oem/a.yaml /oem/sub/dir/b.yml /usr/local/cloud-config/c.yaml /usr/local/cloud-config/deep/d.yaml /oem/sub/note.txt; do echo "--- $f"; cat $f 2>/dev/null || echo "(deleted)"; done; }
echo "===== BEFORE"; dump
echo "===== RUN"; /kairos-agent phone-home uninstall; echo "rc=$?"
echo "===== AFTER"; dump
