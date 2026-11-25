#!/usr/bin/env python3
"""Small QEMU/serial driver for the 6222 timing measurements.

Boots a Kairos ISO or installed disk under KVM + UEFI (OVMF), with the virtio
disk throttled by a named profile, and drives the serial console.
Every serial line is logged with a host timestamp (seconds since QEMU launch).
"""
import json, os, re, shlex, signal, socket, subprocess, sys, threading, time, base64, glob

ROOT = "/home/itxaka/projects/kairos-triage-agent/workspace/.scratch/qa6222b"
TEST_PW = os.environ.get("QA_PW", "<redacted>")
RUN = "/run/user/1000/qa6222b"
OVMF_CODE = "/usr/share/OVMF/x64/OVMF_CODE.4m.fd"
OVMF_VARS = "/usr/share/OVMF/x64/OVMF_VARS.4m.fd"
os.makedirs(RUN, exist_ok=True)

# Disk profiles: QEMU -drive throttling options (bytes/s, ops/s).
PROFILES = {
    "a-unthrottled": {},
    "b-ssd400": {"bps-read": 400e6, "bps-write": 400e6, "iops-total": 30000},
    "c-sd40": {"bps-read": 40e6, "bps-write": 15e6, "iops-total": 1000},
    "d-hdd100-iops150": {"bps-read": 100e6, "bps-write": 100e6, "iops-total": 150},
    "d2-hdd100-seq": {"bps-read": 100e6, "bps-write": 100e6},
    "e-slowsd15": {"bps-read": 15e6, "bps-write": 6e6, "iops-total": 500},
}
CPUS = {
    "host": "host",
    "haswell": "Haswell-noTSX",   # avx2, no sha_ni
    "qemu64": "qemu64",           # no avx, no sha_ni
}


ANSI = re.compile(r"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-9;?=]*[ -/]*[@-~]|\x1b[()][0-9A-B]|\x1b[=>78]")


def throttle_args(profile):
    p = PROFILES[profile]
    return "".join(",throttling.%s=%d" % (k, int(v)) for k, v in p.items())


class VM:
    def __init__(self, name, disk=None, iso=None, cidata=None, profile="a-unthrottled",
                 cpu="host", mem=4096, smp=2, logdir=None, extra=None, cdrom_first=False, fmt="raw"):
        self.fmt = fmt
        self.name, self.disk, self.iso, self.cidata = name, disk, iso, cidata
        self.profile, self.cpu, self.mem, self.smp = profile, cpu, mem, smp
        self.logdir = logdir or os.path.join(ROOT, "vms")
        os.makedirs(self.logdir, exist_ok=True)
        self.extra = extra or []
        self.buf = ""
        self.lines = []  # (t, text)
        self.lock = threading.Lock()
        self.t0 = None
        self.proc = None
        self.sock = None
        self.serial_log = os.path.join(self.logdir, name + ".serial.log")
        self.cmd_log = os.path.join(self.logdir, name + ".cmds.log")
        self.n = 0

    # ---- lifecycle
    def start(self):
        spath = os.path.join(RUN, self.name + ".ser")
        qmp = os.path.join(RUN, self.name + ".qmp")
        self.qmp = qmp
        for f in (spath, qmp):
            if os.path.exists(f):
                os.unlink(f)
        vars_ = os.path.join(RUN, self.name + ".vars.fd")
        subprocess.check_call(["cp", "-f", OVMF_VARS, vars_])
        self.pidfile = os.path.join(RUN, self.name + ".pid")
        args = ["qemu-system-x86_64", "-enable-kvm", "-cpu", CPUS.get(self.cpu, self.cpu),
                "-m", str(self.mem), "-smp", str(self.smp),
                "-drive", "if=pflash,format=raw,readonly=on,file=" + OVMF_CODE,
                "-drive", "if=pflash,format=raw,file=" + vars_,
                "-display", "none", "-vga", "std", "-nodefaults",
                "-netdev", "user,id=n0", "-device", "virtio-net-pci,netdev=n0",
                "-chardev", "socket,id=ser0,path=%s,server=on,wait=off" % spath,
                "-serial", "chardev:ser0",
                "-qmp", "unix:%s,server,nowait" % qmp,
                "-pidfile", self.pidfile, "-daemonize"]
        if self.iso:
            args += ["-drive", "file=%s,format=raw,if=ide,media=cdrom,readonly=on" % self.iso]
        if self.cidata:
            args += ["-drive", "file=%s,format=raw,if=ide,media=cdrom,readonly=on" % self.cidata]
        if self.disk:
            args += ["-drive", "if=none,id=hd0,file=%s,format=%s,cache=none,aio=threads%s" % (self.disk, self.fmt, throttle_args(self.profile)),
                     "-device", "virtio-blk-pci,drive=hd0,serial=T6222"]
        if self.iso:
            args += ["-boot", "d"]
        args += self.extra
        self.args = args
        with open(self.cmd_log, "a") as f:
            f.write("# QEMU: %s\n" % " ".join(shlex.quote(a) for a in args))
        open(self.serial_log, "w").close()
        self.t0 = time.monotonic()
        subprocess.check_call(args)
        self.pid = int(open(self.pidfile).read())
        for _ in range(100):
            try:
                s = socket.socket(socket.AF_UNIX)
                s.connect(spath)
                break
            except OSError:
                time.sleep(0.05)
        else:
            raise RuntimeError("no serial socket")
        self.sock = s
        s.settimeout(0.5)
        self.alive = True
        self.th = threading.Thread(target=self._reader, daemon=True)
        self.th.start()
        return self

    def _reader(self):
        lf = open(self.serial_log, "a")
        pend = ""
        while self.alive:
            try:
                d = self.sock.recv(65536)
            except socket.timeout:
                continue
            except OSError:
                break
            if not d:
                break
            t = time.monotonic() - self.t0
            txt = d.decode("utf-8", "replace").replace("\r", "")
            txt = ANSI.sub("", txt)
            with self.lock:
                self.buf += txt
                if len(self.buf) > 4_000_000:
                    self.buf = self.buf[-2_000_000:]
            pend += txt
            while "\n" in pend:
                l, pend = pend.split("\n", 1)
                lf.write("[%9.3f] %s\n" % (t, l))
                lf.flush()
            # partial line (e.g. login prompt) -> also stamp
            if pend and ("login:" in pend or "Password:" in pend):
                lf.write("[%9.3f] %s\n" % (t, pend)); lf.flush()
                pend = ""
        lf.close()

    def stop(self, sync_first=False):
        self.alive = False
        try:
            os.kill(self.pid, signal.SIGTERM)
        except Exception:
            pass
        for _ in range(100):
            try:
                os.kill(self.pid, 0)
                time.sleep(0.1)
            except OSError:
                break
        else:
            try:
                os.kill(self.pid, signal.SIGKILL)
            except Exception:
                pass
        try:
            self.sock.close()
        except Exception:
            pass

    def running(self):
        try:
            os.kill(self.pid, 0)
            return True
        except OSError:
            return False

    # ---- expect
    def wait(self, regex, timeout=300, consume=True):
        """Wait for regex in unread buffer; returns (match, host_time_since_start)."""
        end = time.monotonic() + timeout
        rx = re.compile(regex, re.M)
        while time.monotonic() < end:
            with self.lock:
                m = rx.search(self.buf)
                if m:
                    t = time.monotonic() - self.t0
                    if consume:
                        self.buf = self.buf[m.end():]
                    return m, t
            time.sleep(0.05)
        raise TimeoutError("%s: timeout %ss waiting for %r; tail=%r" % (self.name, timeout, regex, self.buf[-300:]))

    def send(self, s):
        self.sock.sendall(s.encode() + b"\n")

    def login(self, user="kairos", pw=TEST_PW, timeout=600):
        t_first = None
        for attempt in range(4):
            m, t = self.wait(r"(login: ?$|automatic login)", timeout)
            if t_first is None:
                t_first = t
                self.t_login_prompt = t
            time.sleep(1.5)
            if "automatic" in m.group(0):
                self.send("")
                time.sleep(1)
                break
            self.send(user)
            self.wait(r"Password: ?", 60)
            time.sleep(0.5)
            self.send(pw)
            time.sleep(2.5)
            with self.lock:
                bad = "Login incorrect" in self.buf
                self.buf = ""
            if bad:
                continue
            self.send("sudo -i")
            time.sleep(1.5)
            break
        self.send("stty -echo; export PS1='# '; echo RDY_$((1+1))")
        self.wait(r"RDY_2", 60)
        return t_first

    def sh(self, cmd, timeout=300, log=True):
        self.n += 1
        tag = "%s%d" % (self.name.replace("-", "").replace("_", ""), self.n)
        # marker split so the echoed command never matches
        full = "%s; echo \"__DO\"\"NE_%s_$?\"" % (cmd, tag)
        with self.lock:
            self.buf = ""
        t0 = time.monotonic()
        self.send(full)
        m, _ = self.wait(r"__DONE_%s_(\d+)" % tag, timeout, consume=False)
        with self.lock:
            out = self.buf[:m.start()]
            self.buf = self.buf[m.end():]
        rc = int(m.group(1))
        dt = time.monotonic() - t0
        out = ANSI.sub("", out).strip("\n")
        if log:
            with open(self.cmd_log, "a") as f:
                f.write("$ %s\n%s\n[exit %d, %.2fs]\n\n" % (cmd, out, rc, dt))
        return rc, out

    def put(self, path, content):
        """Write a file in the guest (chunked base64)."""
        b = base64.b64encode(content.encode()).decode()
        self.sh("rm -f %s.b64; : > %s.b64" % (path, path), log=False)
        for i in range(0, len(b), 600):
            self.sh("echo -n %s >> %s.b64" % (b[i:i + 600], path), log=False)
        self.sh("base64 -d %s.b64 > %s; rm -f %s.b64; echo put-ok" % (path, path, path), log=False)

    def shot(self, out_png):
        ppm = out_png.replace(".png", ".ppm")
        s = socket.socket(socket.AF_UNIX); s.connect(self.qmp)
        s.recv(4096)
        s.sendall(b'{"execute":"qmp_capabilities"}\n'); time.sleep(0.2); s.recv(4096)
        s.sendall(json.dumps({"execute": "screendump", "arguments": {"filename": ppm}}).encode() + b"\n")
        time.sleep(1.5); s.close()
        subprocess.call(["magick", ppm, out_png]); os.unlink(ppm)


def mkcidata(path, userdata, name="cidata"):
    d = path + ".d"
    os.makedirs(d, exist_ok=True)
    open(d + "/user-data", "w").write(userdata)
    open(d + "/meta-data", "w").write("instance-id: t6222-%s\n" % name)
    subprocess.check_call(["xorriso", "-as", "mkisofs", "-V", "cidata", "-J", "-R", "-o", path, d],
                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    return path


def record(result_file, **kw):
    kw["ts"] = time.strftime("%Y-%m-%dT%H:%M:%S")
    kw["loadavg"] = open("/proc/loadavg").read().split()[:3]
    with open(result_file, "a") as f:
        f.write(json.dumps(kw) + "\n")
