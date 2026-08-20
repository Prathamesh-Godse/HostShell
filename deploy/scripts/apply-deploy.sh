#!/usr/bin/env bash
# Applies a staged binary and systemd units, then restarts hostshell's
# services. Meant to be run ONLY via the narrowly-scoped sudoers rule
# in deploy/sudoers/hostshell-deploy — never invoked directly by the
# deploy user without sudo.
#
# IMPORTANT: this file must be owned by root with no write access for
# the deploy user (chown root:root, chmod 755). If the deploy user can
# edit this script, a sudoers rule that lets them run "this script" as
# root is equivalent to unrestricted root access — the whole point of
# scoping sudo to one script is defeated if that script is mutable by
# the account being restricted.
set -euo pipefail

STAGE=/opt/hostshell/_incoming

install -m 755 "$STAGE/hostshell" /opt/hostshell/hostshell
cp "$STAGE/hostshell-ssh.service" /etc/systemd/system/hostshell-ssh.service
cp "$STAGE/hostshell-ttyd.service" /etc/systemd/system/hostshell-ttyd.service

systemctl daemon-reload
systemctl restart hostshell-ssh
systemctl restart hostshell-ttyd

# Fail the deploy loudly if either service didn't come back up healthy,
# rather than reporting success while the site is actually down.
systemctl is-active --quiet hostshell-ssh
systemctl is-active --quiet hostshell-ttyd

echo "deploy applied: $(date -Iseconds)"
