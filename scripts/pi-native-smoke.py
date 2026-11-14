#!/usr/bin/env python3
"""Opt-in installed Pi native wake through the standalone Poke launcher.
No model account/network provider; owns all state and child processes.
"""
import json
import os
from pathlib import Path
import queue
import shutil
import subprocess
import tempfile
import threading
import time

PROVIDER = '''
import { createAssistantMessageEventStream } from "@earendil-works/pi-ai/compat";
export default function(pi) {
  pi.registerProvider("kanbi-offline", {
    baseUrl: "http://invalid.local", apiKey: "test-only", api: "openai-completions",
    models: [{ id: "fixture", name: "fixture", reasoning: false, input: ["text"],
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      contextWindow: 32768, maxTokens: 1024 }],
    streamSimple(model) {
      const stream = createAssistantMessageEventStream();
      const message = {
        role: "assistant", content: [{ type: "text", text: "KANBI_NATIVE_WAKE_OK" }],
        api: model.api, provider: model.provider, model: model.id,
        usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0,
          cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
        stopReason: "stop", timestamp: Date.now(),
      };
      queueMicrotask(() => {
        stream.push({ type: "start", partial: message });
        stream.push({ type: "done", reason: "stop", message });
        stream.end();
      });
      return stream;
    },
  });
}
'''

def main():
    if os.environ.get("POKE_NATIVE_PI_TESTS") != "1":
        print("Opt in: POKE_NATIVE_PI_TESTS=1 python3 scripts/pi-native-smoke.py")
        return
    repo = Path(__file__).resolve().parents[1]
    root = Path(tempfile.mkdtemp(prefix="poke-native-pi-"))
    binary = str(root / "poke")
    env = {k: v for k, v in os.environ.items() if not k.startswith(("POKE_", "KANBI_"))}
    env.update(POKE_DB=str(root / "mail.db"), PI_CODING_AGENT_DIR=str(root / "pi"))
    process = None
    success = False
    try:
        subprocess.run(["go", "build", "-o", binary, "./cmd/poke"], cwd=repo, env=env, check=True)
        def cli(*args, identity=None):
            callenv = dict(env)
            if identity is not None:
                callenv["POKE_SESSION_ID"] = str(identity)
            return json.loads(subprocess.check_output([binary, *args], env=callenv, text=True, timeout=10))
        sender = cli("register", "builder")["id"]
        receiver = cli("register", "reviewer", "--harness", "pi")["id"]
        message = cli("send", "reviewer", "--key", "native-pi", "--body", "native wake probe", identity=sender)
        provider = root / "provider.ts"
        provider.write_text(PROVIDER)
        with open(root / "stderr.log", "w") as stderr:
            process = subprocess.Popen([binary, "launch", "reviewer", "--", "pi", "--mode", "rpc",
                "--no-extensions", "-e", str(provider), "--provider", "kanbi-offline", "--model", "fixture",
                "--session-dir", str(root / "sessions")], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                stderr=stderr, env=env, text=True, cwd=root, start_new_session=True)
            lines = queue.Queue()
            def read_events():
                for line in process.stdout:
                    lines.put(line)
            threading.Thread(target=read_events, daemon=True).start()
            events = []
            deadline = time.monotonic() + 25
            while time.monotonic() < deadline:
                try:
                    line = lines.get(timeout=.25)
                except queue.Empty:
                    if process.poll() is not None:
                        break
                    continue
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue
                events.append(event)
                if event.get("type") == "agent_end":
                    break
            (root / "events.json").write_text(json.dumps(events))
            assert any(e.get("type") == "agent_start" for e in events), "native agent never woke"
            assert any(e.get("type") == "agent_end" for e in events), "native turn did not finish"
            deadline = time.monotonic() + 6
            while time.monotonic() < deadline:
                status = cli("status", str(message["id"]), identity=receiver)
                if status["state"] == "notified":
                    break
                time.sleep(.1)
            assert status["state"] == "notified", "native receipt was not persisted"
            cli("ack", str(message["id"]), identity=receiver)
            cli("reply", str(message["id"]), "--key", "reply", "--body", "native wake observed", identity=receiver)
            assert cli("inbox", identity=sender)[0]["body"] == "native wake observed"
            print("PASS: standalone Poke launch, installed Pi native wake, receipt, ack and reply (offline model)")
            success = True
    finally:
        if process is not None:
            import signal
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                process.wait(5)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait()
        if success:
            shutil.rmtree(root)
        else:
            print("Failure artifacts retained:", root)
            log = root / "stderr.log"
            if log.exists():
                print(log.read_text()[-3000:])

if __name__ == "__main__":
    main()
