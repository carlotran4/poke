import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, readFileSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

async function fixture(t, legacy = false) {
  const root = mkdtempSync(join(tmpdir(), 'poke-pi-'));
  const savedEnv = { ...process.env };
  const cli = join(root, 'cli');
  const log = join(root, 'calls');
  writeFileSync(cli, `#!${process.execPath}
import('node:fs').then(async fs => {
 let args=process.argv.slice(2);if(args[0]==='messages')args.shift();
 fs.appendFileSync(process.env.POKE_TEST_LOG, args.join(' ')+'\\n');
 if(args[0]==='pending') { if(process.env.POKE_TEST_DELAY) await new Promise(r=>setTimeout(r,150));console.log(JSON.stringify([{id:17,sender:2,state:'queued',body:'DO_NOT_INJECT_BODY'}])); }
 else if(args[0]==='claim')console.log('{"claimed":true}');
 else console.log('{}');
});
`, {mode: 0o700});
  Object.assign(process.env, {
    POKE_PROTOCOL: legacy ? 'KANBI_MAILBOX_CLI' : 'POKE_CLI',
    POKE_CLI: legacy ? '/stale/poke' : cli,
    POKE_DB: '/poke/mail.db', POKE_SESSION_ID: '7',
    POKE_ENDPOINT_FILE: join(root, 'poke.json'),
    KANBI_MAILBOX_CLI: legacy ? cli : '/stale/kanbi',
    KANBI_DB: '/kanbi/mail.db', KANBI_SESSION_ID: '8',
    KANBI_MESSAGE_ENDPOINT_FILE: join(root, 'kanbi.json'),
    POKE_TEST_LOG: log,
  });
  delete process.env.POKE_MESSAGE_TYPE;
  delete process.env.POKE_TEST_DELAY;
  const handlers = {};
  let poll, text = '', idle = true, pending = false, native = 'native-1';
  const entries = [], sent = [];
  const ctx = {
    mode: 'tui', isIdle: () => idle, hasPendingMessages: () => pending,
    ui: { getEditorText: () => text },
    sessionManager: { getSessionId: () => native, getSessionFile: () => '/persistent/session', getEntries: () => entries },
  };
  const pi = {
    on: (event, fn) => { handlers[event] = fn; },
    sendMessage: (message, opts) => {
      sent.push({message, opts});
      entries.push({ type: 'custom_message', customType: message.customType, details: message.details });
    },
  };
  const originalInterval = globalThis.setInterval;
  globalThis.setInterval = fn => { poll = fn; return { unref() {} }; };
  const {default: extension} = await import(`./pi.mjs?fixture=${root}`);
  try { extension(pi); handlers.session_start({}, ctx); }
  finally { globalThis.setInterval = originalInterval; }
  t.after(() => {
    handlers.session_shutdown();
    for (const key of Object.keys(process.env)) if (!(key in savedEnv)) delete process.env[key];
    Object.assign(process.env, savedEnv);
    rmSync(root, {recursive:true, force:true});
  });
  return {root, log, ctx, sent, entries, poll:()=>poll(),
    editor:value=>{text=value}, idle:value=>{idle=value}, pending:value=>{pending=value},
    conversation:value=>{native=value}, start:()=>handlers.session_start({},ctx),
    calls:()=>existsSync(log)?readFileSync(log,'utf8'):''};
}

test('native receiver preserves drafts and busy turns; uses the selected host protocol', async t => {
  for (const legacy of [false,true]) await t.test(String(legacy), async t => {
    const f=await fixture(t,legacy);
    f.editor('unsent draft');await f.poll();assert.equal(f.calls(),'');
    f.editor('');f.idle(false);await f.poll();assert.equal(f.calls(),'');
    f.idle(true);f.pending(true);await f.poll();assert.equal(f.calls(),'');
    f.pending(false);await f.poll();
    assert.equal(f.sent.length,1);assert.equal(f.sent[0].opts.deliverAs,'followUp');
    assert.equal(f.sent[0].opts.triggerTurn,true);
    assert.ok(!f.sent[0].message.content.includes('DO_NOT_INJECT_BODY'));
    assert.ok(f.calls().includes('claim 17\n'));assert.ok(f.calls().includes('notified 17\n'));
    const registration=JSON.parse(readFileSync(join(f.root,legacy?'kanbi.json':'poke.json')));
    assert.equal(registration.session_id,legacy?8:7);
  });
});
test('native receiver rechecks drafts after asynchronous inbox lookup', async t => {
  const f=await fixture(t);process.env.POKE_TEST_DELAY='1';const done=f.poll();
  while(!f.calls().includes('pending'))await new Promise(r=>setTimeout(r,5));
  f.editor('new draft');await done;assert.equal(f.sent.length,0);assert.ok(!f.calls().includes('claim'));
});
test('persisted native entries reconcile uncertain claims without a second wake', async t => {
  const f=await fixture(t);f.entries.push({type:'custom_message',customType:'kanbi-mailbox',details:{messageId:17}});
  await f.poll();assert.equal(f.sent.length,0);assert.ok(f.calls().includes('notified 17'));assert.ok(!f.calls().includes('claim'));
});
test('conversation change disables the previous address',async t=>{
  const f=await fixture(t);f.conversation('native-2');f.start();
  assert.equal(JSON.parse(readFileSync(join(f.root,'poke.json'))).disabled,true);
  await f.poll();assert.equal(f.calls(),'');
});
