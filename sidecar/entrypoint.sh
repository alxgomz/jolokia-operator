#!/bin/sh
set -e

JOLOKIA_MNT=${JOLOKIA_MNT:=/tmp}

# Check the target dir is a mount point
if ! mountpoint -q $JOLOKIA_MNT; then
  echo "Error: $JOLOKIA_MNT is not a mount point. Please mount a volume to $JOLOKIA_MNT."
  exit 1
fi

cp /jolokia-agent.jar $JOLOKIA_MNT

# Wait for a java process to be available
I=0
while [ $I -lt 12 ]; do
  JPROC=$(java -jar ${JOLOKIA_MNT}/jolokia-agent.jar list | tail -1)
  if [ -n "$JPROC" ]; then
    echo "Found Java process: $JPROC"
    java -jar ${JOLOKIA_MNT}/jolokia-agent.jar $@ start ${JPROC%% *}
    if [ $? -eq 0 ]; then
      while java -jar ${JOLOKIA_MNT}/jolokia-agent.jar status ${JPROC%% *} | grep -q "started for PID ${JPROC%% *}"; do
	sleep 5
      done
    fi
  else
    echo "No Java process found. Retrying..."
    I=$((I + 1))
    sleep 5
  fi
done
