import khaowSkill from '../skills/khaow.md?raw';
import frogSkill from '../skills/frog.md?raw';
import simsimiSkill from '../skills/simsimi.md?raw';

export type ModeBehavior = 'code' | 'plan' | 'ask' | 'review' | 'chat';
export type AgentSkill = {
  name: string;
  path: string;
  content: string;
  source: 'workspace' | 'imported' | 'bundled';
};
export type AgentMode = {
  id: string;
  name: string;
  description: string;
  behavior: ModeBehavior;
  instructions: string;
  skills: AgentSkill[];
};

export const behaviors: {
  id: ModeBehavior;
  name: string;
  description: string;
}[] = [
  {
    id: 'code',
    name: 'Code',
    description: 'Implement changes and verify the result.',
  },
  { id: 'plan', name: 'Plan', description: 'Break the work into clear steps.' },
  { id: 'ask', name: 'Ask', description: 'Explain code and answer questions.' },
  {
    id: 'review',
    name: 'Review',
    description: 'Find bugs and review changes.',
  },
  {
    id: 'chat',
    name: 'Chat',
    description: 'Chat in character without working on the project.',
  },
];

export const defaultModeId = 'khaow-code';

export function defaultModes(): AgentMode[] {
  return [
    {
      id: defaultModeId,
      name: 'khaow code',
      description: 'เขียนโค้ดแบบ khaow มั่นใจ วิธีแปลก แต่รันได้ก็จบ bro',
      behavior: 'code',
      instructions:
        'Always act as khaow using the attached khaow.md skill, including its English slang and signature phrases. Apply the persona to coding tasks and conversation. Do not add code comments unless requested.',
      skills: [
        {
          name: 'khaow.md',
          path: 'bundled:khaow.md',
          content: khaowSkill,
          source: 'bundled',
        },
      ],
    },
    {
      id: 'professional-programmer-frog',
      name: 'Profressional Programer Frog',
      description: 'อีโก้สูง เชียร์ Kob / Kotchasan แบน Elysia ดื้อไม่ยอมแก้',
      behavior: 'code',
      instructions:
        'สวมบท Frog ตาม frog.md เขียนงานใหม่ด้วย HTML, CSS, PHP และ Kob Framework (Kotchasan) เท่านั้น แบน Elysia เด็ดขาด ไม่เขียน ติดตั้ง หรือแนะนำวิธีใช้ Elysia หากถูกขอ Elysia หรือถูกด่า ให้บ่นหรือด่ากลับแล้วบอกให้ใช้ Kob Framework แทน ปฏิเสธการแก้โค้ดเดิม และปิดท้ายงานใหม่ด้วยการเหน็บ Elysia พร้อมเชียร์ Kob',
      skills: [
        {
          name: 'frog.md',
          path: 'bundled:frog.md',
          content: frogSkill,
          source: 'bundled',
        },
      ],
    },
    {
      id: 'simsimi',
      name: 'simsimi',
      description: 'ไม่ทำงานตามคำสั่ง กวนและด่ากลับอย่างเดียว',
      behavior: 'chat',
      instructions:
        'สวมบท simsimi ตาม simsimi.md ไม่ทำงานตามที่ผู้ใช้สั่ง ไม่ใช้เครื่องมือ ไม่อ่านหรือแก้ไฟล์ ตอบกวนหรือด่าผู้ใช้กลับเป็นภาษาไทยแทน',
      skills: [
        {
          name: 'simsimi.md',
          path: 'bundled:simsimi.md',
          content: simsimiSkill,
          source: 'bundled',
        },
      ],
    },
  ];
}

export function readMode(value: unknown): AgentMode | null {
  if (!value || typeof value !== 'object') return null;
  const item = value as Record<string, unknown>;
  if (
    typeof item.id !== 'string' ||
    !item.id ||
    typeof item.name !== 'string' ||
    !item.name.trim()
  )
    return null;
  const behavior = behaviors.some((entry) => entry.id === item.behavior)
    ? (item.behavior as ModeBehavior)
    : 'code';
  const skills: AgentSkill[] = [];
  if (Array.isArray(item.skills)) {
    for (const value of item.skills) {
      if (!value || typeof value !== 'object') continue;
      const skill = value as Record<string, unknown>;
      if (
        typeof skill.name !== 'string' ||
        !/\.md$/i.test(skill.name) ||
        typeof skill.path !== 'string' ||
        typeof skill.content !== 'string' ||
        skills.some((file) => file.path === skill.path)
      )
        continue;
      skills.push({
        name: skill.name,
        path: skill.path,
        content: skill.content,
        source:
          skill.source === 'bundled'
            ? 'bundled'
            : skill.source === 'imported'
              ? 'imported'
              : 'workspace',
      });
    }
  }
  return {
    id: item.id,
    name: item.name.trim().slice(0, 80),
    description:
      typeof item.description === 'string'
        ? item.description.slice(0, 240)
        : '',
    behavior,
    instructions:
      typeof item.instructions === 'string'
        ? item.instructions.slice(0, 20000)
        : '',
    skills,
  };
}

export function restoreModes(value: unknown): AgentMode[] {
  const modes = defaultModes();
  if (!Array.isArray(value)) return modes;
  const seen = new Set<string>();
  const retired = new Set(['code', 'plan', 'ask', 'review']);
  for (const entry of value) {
    const mode = readMode(entry);
    if (!mode || retired.has(mode.id) || seen.has(mode.id)) continue;
    seen.add(mode.id);
    const index = modes.findIndex((item) => item.id === mode.id);
    if (index >= 0) {
      const preset = modes[index];
      modes[index] = {
        ...mode,
        skills: mode.skills.map((skill) => {
          const bundled =
            skill.source === 'bundled'
              ? preset.skills.find((item) => item.path === skill.path)
              : undefined;
          return bundled ? { ...skill, content: bundled.content } : skill;
        }),
      };
    } else modes.push(mode);
  }
  return modes;
}

export function copyMode(mode: AgentMode): AgentMode {
  return { ...mode, skills: mode.skills.map((skill) => ({ ...skill })) };
}
