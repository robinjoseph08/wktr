# Named multiplexer backends, not configurable command templates

wktr drives tmux by shelling out directly. To support herdr, each supported Multiplexer gets a named Go backend behind a single interface (create Window, split Pane, run/prime commands, focus, kill, find), selected by the `multiplexer` config key. Users cannot supply their own command strings.

## Considered Options

User-configurable command templates in YAML (`new_window: "herdr tab create --label {name} ..."`) were rejected: herdr addresses tabs and panes by opaque IDs returned as JSON from prior commands, sizes splits by ratio where tmux uses absolute lines, and Window lookup requires parsing structured output. Templates with placeholders cannot express ID threading, per-multiplexer size math, or output parsing, so they would deliver a broken herdr integration and an unmaintainable config surface. Supporting a new multiplexer is deliberately a code change, not a config entry.
