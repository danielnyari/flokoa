# Capability artifact image (runtime contract §4, roadmap 09).
#
# Built by ../capabilities/build.sh with a per-fixture build context whose
# only content is wheelhouse/ (wheels + manifest.json). busybox supplies the
# static shell + cp the initContainer copy path needs; nothing else.
FROM busybox:stable-musl

ARG CAPABILITY_NAME
ARG CAPABILITY_VERSION
ARG CONTRACT_VERSION=1

LABEL ai.flokoa.capability-name="${CAPABILITY_NAME}" \
      ai.flokoa.capability-version="${CAPABILITY_VERSION}" \
      ai.flokoa.contract-version="${CONTRACT_VERSION}"

# World-readable: the initContainer copies as uid 65532 and the runner reads
# as uid 65532 (runtime contract §4). --chmod applies to the directory too,
# and a 0644 directory is not traversable — restore 0755 on the dir itself.
COPY --chmod=0644 wheelhouse/ /wheelhouse/
RUN chmod 0755 /wheelhouse
