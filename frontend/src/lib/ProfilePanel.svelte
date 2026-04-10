<script>
    import { createQuery, createMutation, useQueryClient } from "@tanstack/svelte-query";
    import { getProfile, putProfile } from "./api.js";
    import { autosize } from "./autosize.js";
    import { showError } from "./toast.js";

    let { onClose } = $props();

    const queryClient = useQueryClient();

    const profileQuery = createQuery(() => ({
        queryKey: ["profile"],
        queryFn: getProfile,
    }));

    const saveMutation = createMutation(() => ({
        mutationFn: (data) => putProfile(data),
        onSuccess: (data) => {
            queryClient.setQueryData(["profile"], data);
            onClose();
        },
        onError: (err) => showError(err, "Failed to save profile."),
    }));

    let gender = $state("");
    let birthYear = $state("");
    let height = $state("");
    let weight = $state("");
    let notes = $state("");
    let goals = $state("");
    let dietaryRestrictions = $state("");

    // Populate fields when profile data loads
    $effect(() => {
        const p = profileQuery.data;
        if (p) {
            gender = p.gender ?? "";
            birthYear = p.birth_year ?? "";
            height = p.height ?? "";
            weight = p.weight ?? "";
            notes = p.notes ?? "";
            goals = p.goals ?? "";
            dietaryRestrictions = p.dietary_restrictions ?? "";
        }
    });

    function save() {
        saveMutation.mutate({
            gender,
            birth_year: birthYear,
            height,
            weight,
            notes,
            goals,
            dietary_restrictions: dietaryRestrictions,
        });
    }

    let saving = $derived(saveMutation.isPending);
    let loaded = $derived(profileQuery.isSuccess);

    function onKeyDown(e) {
        if (e.key === "Escape") onClose();
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) save();
    }
</script>

<svelte:window onkeydown={onKeyDown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" aria-hidden="true" onclick={onClose}></div>
<div
    class="panel"
    role="dialog"
    aria-modal="true"
    aria-labelledby="profile-title"
>
    <div class="panel-header">
        <h2 id="profile-title">Profile</h2>
        <button class="close" onclick={onClose}>✕</button>
    </div>
    <p class="hint-text">
        This information helps the AI estimate macros more accurately.
    </p>

    {#if loaded}
        <div class="fields">
            <label>
                <span>Gender</span>
                <input
                    class="text-entry"
                    type="text"
                    bind:value={gender}
                    placeholder="e.g. male, female, non-binary"
                    disabled={saving}
                />
            </label>
            <label>
                <span>Birth year</span>
                <input
                    class="text-entry"
                    type="text"
                    inputmode="numeric"
                    bind:value={birthYear}
                    placeholder="e.g. 1990"
                    disabled={saving}
                />
            </label>
            <label>
                <span>Height</span>
                <input
                    class="text-entry"
                    type="text"
                    bind:value={height}
                    placeholder="e.g. 5'10&quot; or 178cm"
                    disabled={saving}
                />
            </label>
            <label>
                <span>Weight</span>
                <input
                    class="text-entry"
                    type="text"
                    bind:value={weight}
                    placeholder="e.g. 170lbs or 77kg"
                    disabled={saving}
                />
            </label>
            <label>
                <span>Notes</span>
                <textarea
                    class="text-entry"
                    use:autosize
                    bind:value={notes}
                    placeholder="Dietary restrictions, allergies…"
                    rows="3"
                    disabled={saving}
                ></textarea>
            </label>
            <label>
                <span>Goals</span>
                <textarea
                    class="text-entry"
                    use:autosize
                    bind:value={goals}
                    placeholder="e.g. lose weight, build muscle, eat more protein…"
                    rows="3"
                    disabled={saving}
                ></textarea>
            </label>
            <label>
                <span>Dietary Restrictions</span>
                <textarea
                    class="text-entry"
                    use:autosize
                    bind:value={dietaryRestrictions}
                    placeholder="e.g. vegetarian, no gluten, lactose intolerant…"
                    rows="2"
                    disabled={saving}
                ></textarea>
            </label>
        </div>
        <div class="actions">
            <button class="save-btn" onclick={save} disabled={saving}
                >{saving ? "Saving…" : "Save"}</button
            >
            <button class="cancel-btn" onclick={onClose} disabled={saving}
                >Cancel</button
            >
        </div>
    {:else}
        <p class="status">Loading…</p>
    {/if}
</div>

<style>
    .overlay {
        position: fixed;
        inset: 0;
        background: rgba(0, 0, 0, 0.2);
        z-index: 20;
    }

    .panel {
        position: fixed;
        top: 50%;
        left: 50%;
        transform: translate(-50%, -50%);
        background: var(--paper);
        border-radius: var(--r-md);
        width: min(92vw, 420px);
        max-height: 80vh;
        overflow-y: auto;
        z-index: 21;
        padding: 1.5rem;
        box-shadow: 0 4px 24px rgba(0, 0, 0, 0.12);
    }

    .panel-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: 0.5rem;
    }

    .panel-header h2 {
        font-size: var(--t-title);
        font-weight: 600;
        color: var(--ink);
    }

    .close {
        background: none;
        border: none;
        font-size: 1rem;
        color: var(--mute);
        cursor: pointer;
        padding: 0.25rem;
        line-height: 1;
    }

    .hint-text {
        font-size: var(--t-meta);
        color: var(--mute);
        margin-bottom: 1.25rem;
        line-height: 1.5;
    }

    .fields {
        display: flex;
        flex-direction: column;
        gap: 1rem;
    }

    label {
        display: flex;
        flex-direction: column;
        gap: 0.3rem;
    }

    label span {
        font-size: 0.68rem;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: var(--mute);
        font-weight: 600;
    }

    input,
    textarea {
        border: none;
        border-bottom: 2px solid var(--ink-2);
        padding: 0.3rem 0;
        font-size: var(--t-body);
        font-family: inherit;
        background: transparent;
        color: var(--ink);
        outline: none;
        resize: vertical;
    }

    .status {
        font-size: 0.78rem;
        color: var(--mute-2);
        margin-top: 0.75rem;
    }

    .actions {
        display: flex;
        gap: 0.5rem;
        margin-top: 1.25rem;
    }

    .save-btn {
        flex: 1;
        padding: 0.6rem 1rem;
        background: var(--ink-2);
        color: var(--paper);
        border: none;
        border-radius: var(--r-sm);
        cursor: pointer;
        font-size: var(--t-body-sm);
        font-family: inherit;
        font-weight: 500;
        touch-action: manipulation;
    }

    .save-btn:hover:not(:disabled) {
        background: var(--ink);
    }

    .save-btn:disabled {
        opacity: 0.5;
        cursor: default;
    }

    .cancel-btn {
        padding: 0.6rem 1rem;
        background: none;
        color: var(--mute);
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        cursor: pointer;
        font-size: var(--t-body-sm);
        font-family: inherit;
        touch-action: manipulation;
    }

    .cancel-btn:hover:not(:disabled) {
        border-color: var(--mute);
    }
</style>
