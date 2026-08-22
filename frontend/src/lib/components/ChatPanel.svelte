<script lang="ts">
    import { streamChat, type ChatMessage } from "../models/chat";
    import Icon from "../Icon.svelte";

    interface Props {
        password: string;
        models: { id: string; displayName: string | null }[];
    }

    let { password, models }: Props = $props();

    let model = $state("");
    let draft = $state("");
    let messages = $state<ChatMessage[]>([]);
    let streaming = $state(false);
    let error = $state("");
    let controller: AbortController | null = null;

    const canSend = $derived(
        !streaming && draft.trim().length > 0 && Boolean(model),
    );

    $effect(() => {
        if (!model && models.length > 0) model = models[0].id;
    });

    async function send() {
        const prompt = draft.trim();
        if (!prompt || !model || streaming) return;
        error = "";
        draft = "";
        messages = [
            ...messages,
            { role: "user", content: prompt },
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
            if (!messages[assistantIndex].content) {
                messages = messages.slice(0, -2);
                draft = prompt;
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
</script>

<div class="chat">
    <div class="chat-toolbar">
        <div class="select-field chat-model-select">
            <select bind:value={model} disabled={streaming} aria-label="Model">
                {#each models as item (item.id)}
                    <option value={item.id}
                        >{item.displayName || item.id}</option
                    >
                {/each}
            </select>
        </div>
        {#if messages.length > 0}
            <button
                type="button"
                class="text-button"
                onclick={clear}
                disabled={streaming}
            >
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
                    {#if message.thinking}
                        <details
                            class="chat-thinking"
                            open={streaming &&
                                index === messages.length - 1 &&
                                !message.content}
                        >
                            <summary>Thinking</summary>
                            <p>{message.thinking}</p>
                        </details>
                    {/if}
                    <p
                        class:pending={streaming &&
                            index === messages.length - 1 &&
                            !message.content}
                    >
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
        color: var(--muted, #667085);
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
        background: var(--primary-soft, #eef0ff);
    }
    .chat-message.assistant {
        align-self: flex-start;
        background: var(--surface-alt, #f5f6fa);
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
        gap: 10px;
        align-items: flex-end;
        background: #fff;
        border: 1px solid var(--outline-variant);
        border-radius: 16px;
        padding: 10px;
        box-shadow: 0 1px 2px rgba(26, 27, 32, 0.05);
    }
    .chat-composer:focus-within {
        border: 2px solid var(--primary);
        box-shadow: 0 0 0 3px rgba(54, 92, 141, 0.08);
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
        color: #1a1b20;
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
        color: #b42318;
        margin: 0;
    }
</style>
