# Libvirt virtual machine monitoring

Beszel can collect agentless lifecycle and performance metrics for KVM/QEMU
guests from libvirt. The feature is opt-in and belongs on the agent running on
the hypervisor host.

## Container installation

Mount libvirt's read-only socket and set `LIBVIRT_SOCKET`:

```yaml
services:
  beszel-agent:
    image: henrygd/beszel-agent
    volumes:
      - /run/libvirt/libvirt-sock-ro:/run/libvirt/libvirt-sock-ro:ro
    environment:
      LIBVIRT_SOCKET: /run/libvirt/libvirt-sock-ro
```

Do not mount `libvirt-sock`. The primary socket permits management operations
equivalent to root access. `libvirt-sock-ro` is sufficient for Beszel's state,
CPU, memory, network, and disk queries.

For a native agent or a remote libvirt connection, set a full URI instead:

```text
LIBVIRT_URI=qemu:///system?socket=/run/libvirt/libvirt-sock-ro
```

If both variables are present, `LIBVIRT_SOCKET` overrides the URI's `socket`
query parameter.

## Guest memory metrics

CPU, state, network, and disk metrics do not require software inside the guest.
Accurate guest memory usage requires the virtio balloon driver and periodic
balloon statistics. Configure a reporting interval for each guest if memory is
missing:

```bash
virsh dommemstat VM_NAME --period 10 --live --config
```

Beszel rejects balloon samples older than five minutes instead of displaying
stale memory as current data. It does not substitute the QEMU process RSS,
because that is host allocation rather than guest-visible memory usage.

## Metric semantics

- CPU is normalized against total host CPU capacity, matching the host chart.
- Guest CPU is also retained normalized against the VM's assigned vCPUs.
- Network traffic is reported from the guest's point of view.
- Disk throughput and capacity are aggregated across the domain's block devices.
- Inactive domains remain visible with their libvirt lifecycle state.
