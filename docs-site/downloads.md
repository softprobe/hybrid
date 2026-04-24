---
title: Downloads
---

<script setup>
import { ref, onMounted } from 'vue'

const cliVersion = ref('')
const wasmVersion = ref('')
const cliStatus = ref('loading')
const wasmStatus = ref('loading')

onMounted(async () => {
  try {
    cliVersion.value = (await fetch('https://storage.googleapis.com/softprobe-published-files/cli/softprobe/version')).text().then(r => r.trim())
    cliStatus.value = 'ok'
  } catch { cliStatus.value = 'error' }

  try {
    wasmVersion.value = (await fetch('https://storage.googleapis.com/softprobe-published-files/agent/proxy-wasm/version')).text().then(r => r.trim())
    wasmStatus.value = 'ok'
  } catch { wasmStatus.value = 'error' }
})

const platforms = [
  { label: 'Linux (x86_64)', arch: 'linux-amd64' },
  { label: 'Linux (ARM64)',  arch: 'linux-arm64' },
  { label: 'macOS (Intel)',  arch: 'darwin-amd64' },
  { label: 'macOS (Apple Silicon)', arch: 'darwin-arm64' },
]
</script>

# Downloads

## softprobe CLI {#cli}

**Version:** `{{ cliVersion }}` <Badge v-if="cliStatus === 'loading'" type="warning" text="loading..." /> <Badge v-if="cliStatus === 'error'" type="danger" text="unavailable" />

**Install:** `curl -fsSL https://docs.softprobe.dev/install/cli.sh | sh`

Direct downloads:

| Platform | File |
|---|---|
| Linux (x86_64) | <a v-if="cliStatus === 'ok'" :href="`https://storage.googleapis.com/softprobe-published-files/cli/softprobe/${cliVersion}/softprobe-linux-amd64`">softprobe-linux-amd64</a> |
| Linux (ARM64) | <a v-if="cliStatus === 'ok'" :href="`https://storage.googleapis.com/softprobe-published-files/cli/softprobe/${cliVersion}/softprobe-linux-arm64`">softprobe-linux-arm64</a> |
| macOS (Intel) | <a v-if="cliStatus === 'ok'" :href="`https://storage.googleapis.com/softprobe-published-files/cli/softprobe/${cliVersion}/softprobe-darwin-amd64`">softprobe-darwin-amd64</a> |
| macOS (Apple Silicon) | <a v-if="cliStatus === 'ok'" :href="`https://storage.googleapis.com/softprobe-published-files/cli/softprobe/${cliVersion}/softprobe-darwin-arm64`">softprobe-darwin-arm64</a> |

## sp-istio-agent WASM {#wasm}

**Version:** `{{ wasmVersion }}` <Badge v-if="wasmStatus === 'loading'" type="warning" text="loading..." /> <Badge v-if="wasmStatus === 'error'" type="danger" text="unavailable" />

| Asset | Download |
|---|---|
| WASM binary | <a v-if="wasmStatus === 'ok'" :href="`https://storage.googleapis.com/softprobe-published-files/agent/proxy-wasm/${wasmVersion}/sp_istio_agent.wasm`">sp_istio_agent.wasm</a> |
| Docker image | `ghcr.io/softprobe/softprobe-proxy:{{ wasmVersion }}` |