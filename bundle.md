---
bundle:
  name: agent-daemon
  version: 1.0.0
  description: Agent daemon job scheduler — schedule and manage shell, claude-code, and amplifier jobs via a local daemon at localhost:7700

tools:
  - module: tool-skills
    source: git+https://github.com/microsoft/amplifier-module-tool-skills@main
    config:
      skills:
        - "@agent-daemon:.amplifier/skills"

context:
  include:
    - agent-daemon:.amplifier/context/agent-daemon-awareness.md
---

# Agent Daemon Bundle
