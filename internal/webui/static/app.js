const accounts = new Map();
const logs = [];

const account = document.getElementById("account");
const issueBtn = document.getElementById("issue-btn");
const redeemBtn = document.getElementById("redeem-btn");
const replayBtn = document.getElementById("replay-btn");
const addClientBtn = document.getElementById("add-client-btn");
const generateClientsBtn = document.getElementById("generate-clients-btn");
const clientCount = document.getElementById("client-count");
const note = document.getElementById("note");
const log = document.getElementById("log");
const users = document.getElementById("users");
const clients = ["alice@example.com", "bob@example.com", "carol@example.com"];

async function post(path, body) {
  const res = await fetch(path, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = await res.json();
  if (!res.ok) {
    throw Object.assign(new Error(data.error || "request_failed"), { data, status: res.status });
  }
  return data;
}

async function loadKey() {
  try {
    const res = await fetch("/v1/issuer/key");
    const key = await res.json();
    note.textContent = `key: ${key.key_id}`;
  } catch {
    note.textContent = "key: unavailable";
  }
}

async function issue() {
  setBusy(true);
  try {
    const current = currentAccount();
    const session = await post("/v1/demo/issue", { account: current });
    const bucket = accountBucket(current);
    bucket.tokens.push({ session, redeemed: false });
    syncButtons();
    write(`issued ${short(session.id)} for ${session.account}; pending=${pendingCount(bucket)} remaining=${session.remaining}`);
  } catch (err) {
    write(`issue failed: ${err.message}`);
  } finally {
    setBusy(false);
    syncButtons();
  }
}

async function redeem() {
  const bucket = accountBucket(currentAccount());
  const state = nextToken(bucket);
  if (!state) return;
  setBusy(true);
  try {
    const out = await post("/v1/demo/redeem", { session_id: state.session.id });
    state.redeemed = true;
    bucket.lastRedeemed = state;
    syncButtons();
    write(`redeemed ${short(out.receipt.token_hash)}; pending=${pendingCount(bucket)}`);
  } catch (err) {
    write(`redeem failed: ${err.message}`);
  } finally {
    setBusy(false);
    syncButtons();
  }
}

async function replay() {
  const bucket = accountBucket(currentAccount());
  const state = bucket.lastRedeemed;
  if (!state) return;
  setBusy(true);
  try {
    await post("/v1/demo/redeem", { session_id: state.session.id });
    write("replay accepted");
  } catch (err) {
    write(`replay rejected: ${err.message}`);
  } finally {
    setBusy(false);
    syncButtons();
  }
}

function setBusy(busy) {
  issueBtn.disabled = busy;
  if (busy) {
    redeemBtn.disabled = true;
    replayBtn.disabled = true;
    return;
  }
  syncButtons();
}

function syncButtons() {
  const bucket = accountBucket(currentAccount());
  redeemBtn.disabled = nextToken(bucket) == null;
  replayBtn.disabled = bucket.lastRedeemed == null;
}

function write(message) {
  logs.push(`${new Date().toLocaleTimeString()}  ${message}`);
  while (logs.length > 8) logs.shift();
  log.textContent = logs.join("\n");
  log.scrollTop = log.scrollHeight;
}

function short(value) {
  if (!value) return "-";
  return value.length > 18 ? `${value.slice(0, 10)}...${value.slice(-6)}` : value;
}

issueBtn.addEventListener("click", issue);
redeemBtn.addEventListener("click", redeem);
replayBtn.addEventListener("click", replay);
addClientBtn.addEventListener("click", addClient);
generateClientsBtn.addEventListener("click", generateClients);

account.addEventListener("input", () => {
  renderClients();
  syncButtons();
});

function currentAccount() {
  return account.value.trim();
}

function accountBucket(name) {
  let bucket = accounts.get(name);
  if (!bucket) {
    bucket = { tokens: [], lastRedeemed: null };
    accounts.set(name, bucket);
  }
  return bucket;
}

function nextToken(bucket) {
  return bucket.tokens.find((item) => !item.redeemed) || null;
}

function pendingCount(bucket) {
  return bucket.tokens.filter((item) => !item.redeemed).length;
}

loadKey();
renderClients();
syncButtons();

function addClient() {
  const name = currentAccount();
  if (!name) {
    write("client name is empty");
    return;
  }
  if (!clients.includes(name)) {
    clients.push(name);
    write(`added ${name}`);
  } else {
    write(`selected ${name}`);
  }
  renderClients();
  syncButtons();
}

function generateClients() {
  const count = Math.max(1, Math.min(10000, Number.parseInt(clientCount.value, 10) || 1));
  clients.length = 0;
  for (let i = 0; i < count; i++) {
    clients.push(`client-${String(i + 1).padStart(5, "0")}@example.com`);
  }
  account.value = clients[0];
  renderClients();
  syncButtons();
  write(`generated ${count} clients`);
}

function selectClient(name) {
  account.value = name;
  renderClients();
  syncButtons();
  write(`selected ${name}`);
}

function renderClients() {
  users.replaceChildren();
  for (const name of clients) {
    const btn = document.createElement("button");
    btn.className = name === currentAccount() ? "user active" : "user";
    btn.dataset.account = name;
    btn.textContent = labelFor(name);
    btn.addEventListener("click", () => selectClient(name));
    users.appendChild(btn);
  }
}

function labelFor(name) {
  return name.split("@")[0] || name;
}
