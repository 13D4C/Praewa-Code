import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { spawn, type ChildProcessByStdio } from 'node:child_process';
import type { Readable, Writable } from 'node:stream';
import { randomUUID } from 'node:crypto';
import { request } from 'node:http';
import { fileURLToPath } from 'node:url';

function agentPreview(): Plugin {
  return {
    name: 'praewa-agent-preview',
    configureServer(server) {
      const token = randomUUID();
      let child: ChildProcessByStdio<Writable, null, Readable> | undefined;
      let ready: Promise<number> | undefined;
      function start() {
        if (ready) return ready;
        ready = new Promise<number>((resolve, reject) => {
          const startedChild = spawn('go', ['run', './cmd/agent-preview'], {
            cwd: fileURLToPath(new URL('..', import.meta.url)),
            env: { ...process.env, PRAEWA_AGENT_PREVIEW_TOKEN: token },
            stdio: ['pipe', 'ignore', 'pipe'],
          });
          child = startedChild;
          let output = '';
          const timer = setTimeout(() => {
            startedChild.stdin.end();
            if (child === startedChild) ready = undefined;
            reject(new Error('Agent backend took too long to start'));
          }, 60000);
          startedChild.stderr.on('data', (chunk) => {
            output += chunk.toString();
            const match = output.match(/PRAEWA_AGENT_PORT=(\d+)/);
            if (match) {
              clearTimeout(timer);
              resolve(Number(match[1]));
            }
            if (output.length > 4000) output = output.slice(-4000);
          });
          startedChild.on('error', (error) => {
            clearTimeout(timer);
            if (child === startedChild) ready = undefined;
            reject(error);
          });
          startedChild.on('exit', () => {
            clearTimeout(timer);
            if (child === startedChild) ready = undefined;
            reject(new Error('Agent backend stopped'));
          });
        });
        return ready;
      }
      server.httpServer?.once('close', () => child?.stdin.end());
      server.middlewares.use('/__praewa_agent', async (incoming, outgoing) => {
        const origin = incoming.headers.origin;
        const allowed = [
          `http://127.0.0.1:${server.config.server.port}`,
          `http://localhost:${server.config.server.port}`,
        ];
        if (
          incoming.method !== 'POST' ||
          !incoming.headers['content-type']?.startsWith('application/json') ||
          (origin && !allowed.includes(origin))
        ) {
          outgoing.writeHead(403);
          outgoing.end('Forbidden');
          return;
        }
        try {
          const port = await start();
          const proxy = request(
            {
              hostname: '127.0.0.1',
              port,
              path: '/__praewa_agent' + incoming.url,
              method: 'POST',
              headers: {
                'Content-Type': 'application/json',
                Authorization: 'Bearer ' + token,
              },
            },
            (response) => {
              outgoing.writeHead(response.statusCode ?? 502, {
                'Content-Type': 'application/json',
                'Cache-Control': 'no-store',
              });
              response.pipe(outgoing);
            },
          );
          proxy.on('error', () => {
            if (!outgoing.headersSent)
              outgoing.writeHead(502, { 'Content-Type': 'application/json' });
            outgoing.end(
              JSON.stringify({
                error: 'Agent backend disconnected. Reconnect and try again.',
              }),
            );
          });
          incoming.pipe(proxy);
        } catch (error) {
          outgoing.writeHead(503, { 'Content-Type': 'application/json' });
          outgoing.end(
            JSON.stringify({
              error:
                error instanceof Error
                  ? error.message
                  : 'Agent backend unavailable',
            }),
          );
        }
      });
    },
  };
}

export default defineConfig({
  plugins: [tailwindcss(), svelte(), agentPreview()],
  clearScreen: false,
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('@xterm')) return 'terminal';
          if (/dompurify|marked/.test(id)) return 'markdown-preview';
          const language = id.match(
            /(?:@codemirror\/lang-|@lezer\/)([a-z]+)/,
          )?.[1];
          if (language && !['common', 'highlight', 'lr'].includes(language))
            return `syntax-${language}`;
          if (id.includes('node_modules') && /codemirror|@lezer/.test(id))
            return 'editor-core';
        },
      },
    },
  },
  server: { strictPort: true, port: 5173 },
});
