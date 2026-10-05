const icons = import.meta.glob<string>('../assets/file-icons/*.svg', {
  eager: true,
  query: '?url',
  import: 'default',
});

const extensions: Record<string, string> = {
  go: 'go',
  js: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  ts: 'typescript',
  mts: 'typescript',
  cts: 'typescript',
  jsx: 'react',
  tsx: 'react-typescript',
  json: 'json',
  jsonc: 'json',
  json5: 'json',
  svelte: 'svelte',
  vue: 'vue',
  py: 'python',
  pyw: 'python',
  rs: 'rust',
  c: 'c',
  h: 'c',
  cpp: 'cpp',
  cc: 'cpp',
  cxx: 'cpp',
  hpp: 'cpp',
  hh: 'cpp',
  cs: 'csharp',
  css: 'css',
  scss: 'sass',
  sass: 'sass',
  html: 'html',
  htm: 'html',
  md: 'markdown',
  mdx: 'markdown',
  yaml: 'yaml',
  yml: 'yaml',
  xml: 'xml',
  toml: 'toml',
  sh: 'shell',
  bash: 'shell',
  zsh: 'shell',
  fish: 'shell',
  bat: 'shell',
  cmd: 'shell',
  ps1: 'powershell',
  psm1: 'powershell',
  psd1: 'powershell',
  sql: 'sql',
  java: 'java',
  kt: 'kotlin',
  kts: 'kotlin',
  swift: 'swift',
  dart: 'dart',
  lua: 'lua',
  php: 'php',
  rb: 'ruby',
  svg: 'svg',
  png: 'image',
  jpg: 'image',
  jpeg: 'image',
  gif: 'image',
  webp: 'image',
  ico: 'image',
  ini: 'config',
  conf: 'config',
  cfg: 'config',
};

const filenames: Record<string, string> = {
  'go.mod': 'go-module',
  'go.sum': 'go-module',
  'go.work': 'go-work',
  'go.work.sum': 'go-work',
  'package.json': 'npm',
  'package-lock.json': 'npm',
  'npm-shrinkwrap.json': 'npm',
  '.npmrc': 'npm',
  '.gitignore': 'git',
  '.gitattributes': 'git',
  '.gitmodules': 'git',
  '.gitconfig': 'git',
  dockerfile: 'docker',
  '.dockerignore': 'docker',
  makefile: 'config',
  gnumakefile: 'config',
};

export function fileIcon(name: string) {
  const basename =
    name.replace(/\\/g, '/').split('/').pop()?.toLowerCase() ?? '';
  const extension = basename.includes('.')
    ? (basename.split('.').pop() ?? '')
    : '';
  const kind =
    filenames[basename] ??
    (basename === '.env' || basename.startsWith('.env.')
      ? 'env'
      : basename.startsWith('dockerfile.')
        ? 'docker'
        : extensions[extension]);
  return kind ? icons[`../assets/file-icons/${kind}.svg`] : undefined;
}
