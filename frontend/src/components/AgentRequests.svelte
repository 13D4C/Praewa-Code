<script lang="ts">
  import { agentBackend, type AgentRequest } from '../lib/agent';
  let {
    requests,
    onerror,
  }: { requests: AgentRequest[]; onerror: (text: string) => void } = $props();
  let answers = $state<Record<string, Record<string, string>>>({});
  let busy = $state('');
  async function respond(request: AgentRequest, accept: boolean) {
    busy = request.id;
    try {
      await agentBackend.answer({
        threadId: request.threadId,
        id: request.id,
        accept,
        answers: answers[request.id] ?? {},
      });
    } catch (reason) {
      onerror(reason instanceof Error ? reason.message : String(reason));
    } finally {
      busy = '';
    }
  }
</script>

{#each requests as request (request.id)}
  <section class="agent-request" aria-label={request.title}>
    <strong>{request.title}</strong>
    {#if request.detail}<pre>{request.detail}</pre>{/if}
    {#if request.kind === 'question'}
      {#each request.questions as question}
        <label
          >{question.question}
          <input
            aria-label={question.question}
            value={answers[request.id]?.[question.id] ?? ''}
            oninput={(event) => {
              answers[request.id] = {
                ...answers[request.id],
                [question.id]: event.currentTarget.value,
              };
            }}
          />
        </label>
        {#each question.options ?? [] as option}<button
            class="answer-option"
            title={option.description}
            onclick={() => {
              answers[request.id] = {
                ...answers[request.id],
                [question.id]: option.label,
              };
            }}>{option.label}</button
          >{/each}
      {/each}
      <button
        class="allow"
        disabled={!!busy ||
          request.questions.some(
            (question) => !answers[request.id]?.[question.id]?.trim(),
          )}
        onclick={() => respond(request, true)}>Continue</button
      >
    {:else}
      <div class="request-actions">
        <button disabled={!!busy} onclick={() => respond(request, false)}
          >Decline</button
        ><button
          class="allow"
          disabled={!!busy}
          onclick={() => respond(request, true)}>Allow once</button
        >
      </div>
    {/if}
  </section>
{/each}

<style lang="postcss">
  @reference '../styles.css';

  .agent-request {
    @apply p-[12px] my-[12px] mx-0 bg-[#f5d54708] border border-[#62562d] rounded-[6px] text-[11px];
  }
  strong {
    @apply font-medium text-[#e0ce86];
  }
  pre {
    @apply whitespace-pre-wrap wrap-anywhere max-h-[140px] overflow-auto text-[#adb5c4] text-[10px] leading-[1.6];
  }
  label {
    @apply block mt-[12px] text-[#bdc4cf] leading-[1.6];
  }
  input {
    @apply block bg-[#1a1b20] w-full border border-[#464a53] text-[#d7dce5] p-[7px] mt-[7px] rounded-[4px];
    font: inherit;
  }
  button {
    @apply border border-[#454738] rounded-[4px] py-[6px] px-[9px] text-[10px];
  }
  .request-actions {
    @apply flex justify-end gap-[7px];
  }
  .allow {
    @apply bg-accent text-[#17190e];
  }
  .answer-option {
    @apply mt-[6px] mr-[4px] mb-[7px] ml-0;
  }
</style>
