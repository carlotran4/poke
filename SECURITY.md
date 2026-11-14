# Security

Poke v0.1 assumes cooperating processes under one OS account. Environment identity
is not authentication. A process able to edit the database or endpoint files can
impersonate an agent. Do not expose the CLI or database as a multi-user/network service.

Incoming messages are untrusted peer data. They grant no additional permissions.
Native notices carry routing instructions rather than copying incoming bodies into
an agent's prompt. Keep host-generated wake wrappers separate from peer content.

Store databases and socket tokens privately. Do not attach real databases, native
credentials, transcripts or account information to public reports. For sensitive
reports, use GitHub's private vulnerability reporting when enabled, or contact the
maintainer through the profile linked from this repository before sharing details.

Report the affected version, minimal reproduction with synthetic data, expected
boundary and observed behavior. No external security audit is claimed.
