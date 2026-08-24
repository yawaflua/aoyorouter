<script lang="ts">
    import { readFile, streamChat, type ChatMessage, type ContentBlock } from "../models/chat";
    import Icon from "../Icon.svelte";

    interface Props {
        password: string;
        models: { id: string; displayName: string | null }[];
    }

    let { password, models }: Props = $props();

    type Attachment = ContentBlock & { name: string };

    let model = $state("");
    let draft = $state("");
    let messages = $state<ChatMessage[]>([]);
    let attachments = $state<Attachment[]>([]);
    let streaming = $state(false);
    let error = $state("");
    let fileInput: HTMLInputElement | null = null;
    let controller: AbortController | null = null;

    const canSend = $derived(
        !streaming && (draft.trim().length > 0 || attachments.length > 0) && Boolean(model),
    );

    $effect(() => {
        if (!model && models.length > 0) model = models[0].id;
    });

    async function pickFiles(event: Event) {
        const input = event.currentTarget as HTMLInputElement;
        const picked = Array.from(input.files ?? []);
        input.value = "";
        for (const file of picked) {
            if (file.size > 20 * 1024 * 1024) {
                error = `${file.name} is too large (max 20 MB).`;
                continue;
            }
            try {
                attachments = [...attachments, await readFile(file)];
            } catch (caught) {
                error = caught instanceof Error ? caught.message : `Could not read ${file.name}.`;
            }
        }
    }

    function removeAttachment(index: number) {
        attachments = attachments.filter((_, i) => i !== index);
    }

    async function send() {
        const prompt = draft.trim();
        if ((!prompt && attachments.length === 0) || !model || streaming) return;
        const sentAttachments = attachments;
        error = "";
        draft = "";
        attachments = [];
        messages = [
            ...messages,
            {
                role: "user",
                content: prompt,
                attachments: sentAttachments.length > 0 ? sentAttachments.map((item) => item.name) : undefined,
            },
            { role: "assistant", content: "" },
        ];
        const assistantIndex = messages.length - 1;

        controller = new AbortController();
        streaming = true;
        try {
            await streamChat(
                password,
                model,
                messages.slice(0, -1),
                sentAttachments,
                {
                    onDelta: (text) => {
                        messages[assistantIndex] = {
                            ...messages[assistantIndex],
                            content: messages[assistantIndex].content + text,
                        };
                    },
                    onThinking: (text) => {
                        const current = messages[assistantIndex];
                        messages[assistantIndex] = {
                            ...current,
                            thinking: (current.thinking ?? "") + text,
                        };
                    },
                    onDone: () => {},
                },
                controller.signal,
            );
        } catch (caught) {
            error =
                caught instanceof Error
                    ? caught.message
                    : "The request failed.";
            if (!messages[assistantIndex].content && !messages[assistantIndex].thinking) {
                messages = messages.slice(0, -2);
                draft = prompt;
                attachments = sentAttachments;
            }
        } finally {
            streaming = false;
            controller = null;
        }
    }

    function stop() {
        controller?.abort();
    }

    function clear() {
        stop();
        messages = [];
        error = "";
    }

    function onKeydown(event: KeyboardEvent) {
        if (event.key === "Enter" && !event.shiftKey) {
            event.preventDefault();
            void send();
        }
    }

    function sortedModels() {
        return models.sort((a, b) => a.id.localeCompare(b.id, undefined, { sensitivity: 'base' }));
    }
</script>

<div class="chat">
    <div class="chat-toolbar">
        <div class="select-field chat-model-select">
            <select bind:value={model} disabled={streaming} aria-label="Model">
                {#each sortedModels() as item (item.id)}
                    <option value={item.id}>{item.displayName || item.id}</option>
                {/each}
            </select>
        </div>
        {#if messages.length > 0}
            <button type="button" class="text-button" onclick={clear} disabled={streaming}>
                <Icon name="trash" size={17} /> Clear
            </button>
        {/if}
    </div>

    <div class="chat-history" aria-live="polite">
        {#if messages.length === 0}
            <div class="chat-empty">
                <div class="state-icon"><Icon name="chat" size={28} /></div>
                <h2>Start a conversation</h2>
                <p>
                    Pick a model and send a message. Requests go through the
                    same endpoints your coding agents use.
                </p>
            </div>
        {:else}
            {#each messages as message, index (index)}
                <article class="chat-message {message.role}">
                    <header>
                        {message.role === "user" ? "You" : "Assistant"}
                    </header>
                    {#if message.attachments?.length}
                        <div class="chat-attachments">
                            {#each message.attachments as name (name)}
                                <span class="attachment-chip"><Icon name="logs" size={14} />{name}</span>
                            {/each}
                        </div>
                    {/if}
                    {#if message.thinking}
                        <details class="chat-thinking" open={streaming && index === messages.length - 1 && !message.content}>
                            <summary>Thinking</summary>
                            <p>{message.thinking}</p>
                        </details>
                    {/if}
                    <p class:pending={streaming && index === messages.length - 1 && !message.content}>
                        {message.content || "…"}
                    </p>
                </article>
            {/each}
        {/if}
    </div>

    {#if error}<p class="form-error" role="alert">
            <Icon name="warning" size={18} />{error}
        </p>{/if}

    <form
        class="chat-composer"
        onsubmit={(event) => {
            event.preventDefault();
            void send();
        }}
    >
        <input
            type="file"
            multiple
            hidden
            accept="image/*,text/*,.json,.md,.csv,.log,.ts,.tsx,.js,.jsx,.py,.go,.rs,.java,.c,.cpp,.h,.css,.html,.yml,.yaml,.toml,.sql,.sh"
            bind:this={fileInput}
            onchange={pickFiles}
        />
        {#if attachments.length > 0}
            <div class="chat-attachments composer">
                {#each attachments as attachment, index (index)}
                    <span class="attachment-chip">
                        {attachment.name}
                        <button type="button" aria-label={`Remove ${attachment.name}`} onclick={() => removeAttachment(index)}>×</button>
                    </span>
                {/each}
            </div>
        {/if}
        <div class="composer-row">
            <button type="button" class="icon-button" aria-label="Attach files" onclick={() => fileInput?.click()} disabled={streaming}>
                <Icon name="plus" size={19} />
            </button>
            <textarea
                rows="3"
                bind:value={draft}
                onkeydown={onKeydown}
                placeholder="Send a message… (Enter to send, Shift+Enter for a new line)"
                disabled={!model}></textarea>
            {#if streaming}
                <button type="button" class="tonal chat-send" onclick={stop}
                    ><Icon name="power" size={19} /> Stop</button
                >
            {:else}
                <button type="submit" class="filled chat-send" disabled={!canSend}
                    ><Icon name="chevron" size={19} /> Send</button
                >
            {/if}
        </div>
    </form>
</div>

<style>
    .chat {
        display: flex;
        flex-direction: column;
        gap: 14px;
        height: calc(100vh - 220px);
        min-height: 420px;
    }
    .chat-toolbar {
        display: flex;
        align-items: center;
        gap: 10px;
    }
    .chat-model-select {
        max-width: 360px;
        width: 100%;
    }
    .chat-model-select select {
        width: 100%;
    }
    .chat-history {
        flex: 1;
        overflow-y: auto;
        display: flex;
        flex-direction: column;
        gap: 12px;
        padding: 4px 2px;
    }
    .chat-empty {
        margin: auto;
        text-align: center;
        display: grid;
        gap: 8px;
        justify-items: center;
        color: var(--muted);
    }
    .chat-message {
        border-radius: 14px;
        padding: 12px 16px;
        max-width: min(760px, 92%);
    }
    .chat-message header {
        font-size: 12px;
        letter-spacing: 0.06em;
        text-transform: uppercase;
        opacity: 0.65;
        margin-bottom: 6px;
    }
    .chat-message p {
        white-space: pre-wrap;
        word-break: break-word;
        margin: 0;
    }
    .chat-message.user {
        align-self: flex-end;
        background: var(--primary-container);
        color: var(--on-primary-container);
    }
    .chat-message.assistant {
        align-self: flex-start;
        background: var(--surface-container);
        color: var(--on-surface);
    }
    p.pending::after {
        content: "▍";
        animation: blink 1s steps(2) infinite;
    }
    @keyframes blink {
        to {
            visibility: hidden;
        }
    }
    .chat-composer {
        display: flex;
        flex-direction: column;
        gap: 8px;
        background: var(--surface-container-lowest);
        border: 1px solid var(--outline-variant);
        border-radius: 16px;
        padding: 10px;
        box-shadow: 0 1px 2px var(--shadow);
    }
    .composer-row {
        display: flex;
        gap: 10px;
        align-items: flex-end;
    }
    .chat-composer .icon-button {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 44px;
        height: 44px;
        border-radius: 12px;
        border: 1px solid var(--outline-variant);
        background: transparent;
        color: var(--muted);
    }
    .chat-composer .icon-button:hover { background: var(--surface-container); }
    .chat-attachments { display: flex; flex-wrap: wrap; gap: 6px; }
    .attachment-chip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 4px 10px;
        border-radius: 999px;
        background: var(--surface-container);
        font-size: 13px;
        max-width: 260px;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .attachment-chip button {
        border: 0;
        background: none;
        color: inherit;
        font-size: 15px;
        line-height: 1;
        padding: 0 2px;
    }
    .chat-composer:focus-within {
        border: 2px solid var(--primary);
        box-shadow: 0 0 0 3px color-mix(in srgb, var(--primary) 12%, transparent);
    }
    .chat-composer textarea {
        flex: 1;
        resize: none;
        min-height: 56px;
        max-height: 220px;
        padding: 8px 10px;
        border: 0;
        outline: 0;
        background: transparent;
        color: var(--on-surface);
        line-height: 1.5;
    }
    .chat-send {
        height: 44px;
        white-space: nowrap;
        border-radius: 12px;
    }
    .chat-thinking {
        margin: 4px 0 8px;
        border-left: 3px solid var(--outline-variant);
        padding: 2px 0 2px 12px;
    }
    .chat-thinking summary {
        cursor: pointer;
        font-size: 13px;
        color: var(--muted);
        user-select: none;
    }
    .chat-thinking p {
        font-size: 14px;
        color: var(--muted);
        white-space: pre-wrap;
        word-break: break-word;
        margin: 6px 0 0;
    }
    .form-error {
        display: flex;
        gap: 6px;
        align-items: center;
        color: var(--error);
        margin: 0;
    }
</style>
