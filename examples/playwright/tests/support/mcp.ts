import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';

// A minimal MCP client, so the agent path is tested the way an agent
// actually reaches it.
//
// Driving the page with Playwright's own API proves the mirror works;
// it does not prove an agent can use it. An agent sees only a snapshot
// and addresses elements by the refs inside it, and that path has its
// own failure modes — a control whose name is ambiguous, an element a
// ref cannot resolve, a field that refuses browser_type. Those are
// invisible to every other spec here.

export class McpSession {
  private srv: ChildProcessWithoutNullStreams;
  private buf = '';
  private id = 0;
  private pending = new Map<number, (m: any) => void>();

  constructor() {
    this.srv = spawn('npx', ['-y', '@playwright/mcp@latest', '--headless'],
                     { stdio: ['pipe', 'pipe', 'ignore'] });
    this.srv.stdout.on('data', (d) => this.consume(String(d)));
  }

  private consume(chunk: string) {
    this.buf += chunk;
    for (let nl; (nl = this.buf.indexOf('\n')) >= 0; ) {
      const line = this.buf.slice(0, nl).trim();
      this.buf = this.buf.slice(nl + 1);
      if (!line) continue;
      let msg: any;
      try { msg = JSON.parse(line); } catch { continue; }
      const resolve = msg.id != null && this.pending.get(msg.id);
      if (resolve) { this.pending.delete(msg.id); resolve(msg); }
    }
  }

  rpc(method: string, params: unknown, timeoutMs = 120_000): Promise<any> {
    const id = ++this.id;
    return new Promise((resolve, reject) => {
      this.pending.set(id, (m) => (m.error ? reject(new Error(JSON.stringify(m.error))) : resolve(m.result)));
      this.srv.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
      setTimeout(() => reject(new Error(`MCP timeout: ${method}`)), timeoutMs);
    });
  }

  async start() {
    await this.rpc('initialize', {
      protocolVersion: '2024-11-05', capabilities: {},
      clientInfo: { name: 'devicedeck-spec', version: '1' },
    });
  }

  /** call returns the tool's text content, which is all an agent gets. */
  async call(name: string, args: Record<string, unknown>): Promise<string> {
    const res = await this.rpc('tools/call', { name, arguments: args });
    return (res?.content || []).map((c: any) => c.text || '').join('\n');
  }

  /** snapshotUntil polls the page until the snapshot matches, as an agent must. */
  async snapshotUntil(match: RegExp, seconds = 60): Promise<string> {
    let snap = '';
    for (let waited = 0; waited < seconds; waited += 2) {
      await this.call('browser_wait_for', { time: 2 });
      snap = await this.call('browser_snapshot', {});
      if (match.test(snap)) return snap;
    }
    throw new Error(`snapshot never matched ${match}:\n${snap}`);
  }

  close() { this.srv.kill(); }
}

/** refFor reads the ref out of the first snapshot line matching re. */
export function refFor(snapshot: string, re: RegExp): string {
  const line = snapshot.split('\n').find((l) => re.test(l));
  const ref = line?.match(/\[ref=(e\d+)\]/)?.[1];
  if (!ref) throw new Error(`no ref for ${re} in snapshot:\n${snapshot}`);
  return ref;
}
