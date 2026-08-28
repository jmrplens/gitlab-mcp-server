import ast
import os
import json
import subprocess
import sys
import threading
import time


def run(env_extra, label, timeout=400):
    env = {"PATH": "/usr/bin:/bin", "HOME": "/root"}
    env.update(env_extra)
    t0 = time.time()
    p = subprocess.Popen(
        [os.environ.get("PROBE_BIN", "/tmp/gms")],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
        text=True,
        bufsize=1,
    )
    init = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2025-06-18",
            "capabilities": {},
            "clientInfo": {"name": "probe", "version": "1"},
        },
    }
    try:
        p.stdin.write(json.dumps(init) + "\n")
        p.stdin.flush()
    except Exception:
        pass
    result = {"label": label, "init_secs": None, "exit": None}
    holder = {}

    def reader():
        holder["line"] = p.stdout.readline()

    th = threading.Thread(target=reader, daemon=True)
    th.start()
    th.join(timeout)
    line = holder.get("line")
    if line:
        result["init_secs"] = round(time.time() - t0, 1)
        try:
            j = json.loads(line)
            result["server"] = j.get("result", {}).get("serverInfo")
        except Exception:
            result["raw"] = line[:200]
    else:
        result["init_secs"] = ">%ds (no response)" % timeout
    rc = p.poll()
    if rc is None:
        p.kill()
    result["exit"] = rc
    try:
        err = p.stderr.read(4000)
    except Exception:
        err = ""
    result["stderr_tail"] = err[-800:]
    print(json.dumps(result, indent=2), flush=True)


if __name__ == "__main__":
    label = sys.argv[1]
    env = ast.literal_eval(sys.argv[2])
    to = int(sys.argv[3]) if len(sys.argv) > 3 else 400
    run(env, label, to)
