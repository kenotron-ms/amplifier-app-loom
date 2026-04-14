# Mic-State Connector — Meeting Lifecycle Triggers for Loom

    ## Goal

    Enable Loom jobs to fire on meeting start and meeting end using system microphone state as the signal — without requiring OpenWhispr or any conferencing app to be installed.

    ---

    ## Background

    Mic activity from other processes is a reliable meeting lifecycle signal. When Zoom, Teams, Google Meet, or any other conferencing app grabs the microphone, the OS audio session state changes. When the meeting ends and the mic is released, the state reverts.

    Loom's connector pattern — poll an external source, diff against a mirror, fire jobs on change — maps well to this. The tradeoff is polling latency (3–5s) vs real-time event-driven detection, which is acceptable for meeting lifecycle automation (creating notes, updating presence, triggering workflows).

    The pattern is proven in production by OpenWhispr, which uses platform-native CoreAudio (macOS), WASAPI (Windows), and pactl (Linux) listeners. See amplifier-specs: `_lifeos/Specs/patterns/meeting-aware-event-triggering.md`.

    ---

    ## Connector Design

    **Entity address:** `system.mic/default`

    **State shape:**
    ```json
    {
      "state": "active | inactive",
      "since": "ISO8601 timestamp of last transition"
    }
    ```

    **Setup:**
    ```bash
    loom connector add \
      --name "mic-state" \
      --method command \
      --command "<platform-command-below>" \
      --entity "system.mic/default" \
      --prompt "Track mic state: { state: 'active' | 'inactive', since: ISO8601 timestamp of last transition }. Active means another application is currently using the microphone." \
      --interval 4s
    ```

    ---

    ## Platform Commands

    ### macOS

    ```bash
    # Count active CoreAudio input audio engines
    # Non-zero = mic in use by another app
    ioreg -l -w 0 -c IOAudioEngine 2>/dev/null \
      | grep -B5 'Input' \
      | grep -c '"IOAudioEngineState" = 1' \
      | awk '{print ($1>0 ? "active" : "inactive")}'
    ```

    ### Linux

    ```bash
    # Count active PulseAudio/PipeWire source-outputs
    pactl list source-outputs short 2>/dev/null \
      | grep -vc '^$' \
      | awk '{print ($1>0 ? "active" : "inactive")}'
    ```

    ### Windows

    The WASAPI per-process binary from OpenWhispr (`windows-mic-listener.exe`) is the reliable path. Without it, a PowerShell fallback using process names degrades to process detection (the anti-pattern — see amplifier-specs pattern).

    ```powershell
    # Degraded fallback only — prefers WASAPI binary if available
    if (Get-Process -Name "zoom","teams","webex","meet" -ErrorAction SilentlyContinue) { "active" } else { "inactive" }
    ```

    ---

    ## Transition Filtering Inside Jobs

    `MIRROR_DIFF_JSON` contains field-level diffs. Filter jobs to only the transition direction you care about:

    ```bash
    # Only act when mic becomes active (meeting started)
    echo "$MIRROR_DIFF_JSON" | jq -e '.[] | select(.path == "state" and .to == "active")' > /dev/null || exit 0

    # Only act when mic goes inactive (meeting ended)
    echo "$MIRROR_DIFF_JSON" | jq -e '.[] | select(.path == "state" and .to == "inactive")' > /dev/null || exit 0
    ```

    ---

    ## Example Jobs

    ### Meeting Started — Create a LifeOS Note

    ```bash
    loom add \
      --name "meeting-started-note" \
      --trigger connector \
      --connector-id <mic-state-id> \
      --executor amplifier \
      --prompt "Check MIRROR_DIFF_JSON. If mic state changed to 'active', create a new meeting note in the LifeOS vault under Work/Notes with today's date and title 'Meeting HH:MM'. Leave body blank for transcription."
    ```

    ### Meeting Ended — Prompt for Summary

    ```bash
    loom add \
      --name "meeting-ended-summarize" \
      --trigger connector \
      --connector-id <mic-state-id> \
      --executor amplifier \
      --prompt "Check MIRROR_DIFF_JSON. If mic state changed to 'inactive', find the most recent meeting note created today in Work/Notes and add ## Summary and ## Action Items placeholder sections if missing."
    ```

    ### Presence (macOS)

    ```bash
    loom add \
      --name "meeting-set-focus" \
      --trigger connector \
      --connector-id <mic-state-id> \
      --executor shell \
      --command 'echo "$MIRROR_DIFF_JSON" | jq -e ".[0] | select(.path=="state" and .to=="active")" && shortcuts run "Enable Focus Mode" || true'
    ```

    ---

    ## Sustain Window Limitation

    Polling fires on every detected transition without a sustain window. At a 4s poll interval:

    - **False starts**: a sub-4s mic grab between polls may not register — acceptable for note creation
    - **Mute/unmute during meetings**: a brief mute (< 4s) will not trigger "ended" — acceptable

    For workflows where false-end triggers are costly (e.g. archiving a note), guard on `since` duration:

    ```bash
    # Guard: only act if inactive state has persisted 60+ seconds
    SINCE=$(echo "$MIRROR_CURR_JSON" | jq -r '.since')
    SECS=$(( $(date +%s) - $(date -j -f "%Y-%m-%dT%H:%M:%SZ" "$SINCE" +%s 2>/dev/null || date -d "$SINCE" +%s) ))
    [ "$SECS" -lt 60 ] && exit 0
    ```

    ---

    ## Future: Native Event-Driven Trigger

    The polling approach trades latency for simplicity. A native `mic` trigger type in Loom would use the same platform binaries (already proven in OpenWhispr) and deliver real-time transitions with configurable sustain windows:

    ```bash
    # Hypothetical future API
    loom add \
      --name "meeting-started" \
      --trigger mic \
      --on active \
      --sustain 2s \
      --executor amplifier \
      --prompt "Meeting just started. Create a meeting note."
    ```

    The binary protocol exists and is production-proven. The integration work: embed the binary as a Loom plugin, implement a `mic` trigger type that subscribes to binary stdout, apply sustain logic before firing. Until then, the connector polling approach is the pragmatic path.
    