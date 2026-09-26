import { expect, test } from "bun:test"
import { classifyVersionChange } from "./package-updates"

// version pairs are taken from real apt, dnf, zypper, pacman and apk output
test("major, minor and patch use the first differing upstream component", () => {
	expect(classifyVersionChange("1.10.7-1", "2.0.0-1")).toBe("major")
	expect(classifyVersionChange("1.10.7-1", "1.11.0-1")).toBe("minor")
	expect(classifyVersionChange("3.0.7-104.el9", "3.1.5-8.el9")).toBe("minor")
	expect(classifyVersionChange("1.3.7-1", "1.3.8-1")).toBe("patch")
	expect(classifyVersionChange("0.21.7-1", "0.21.8.2-1")).toBe("patch")
	expect(classifyVersionChange("3.3.0-r2", "3.3.7-r0")).toBe("patch")
	expect(classifyVersionChange("0.7.36-2.fc42", "0.7.37-2.fc42")).toBe("patch")
	// missing components count as zero
	expect(classifyVersionChange("1.2", "1.2.1")).toBe("patch")
	expect(classifyVersionChange("1.2", "1.3.0")).toBe("minor")
	// components compare as numbers, not strings
	expect(classifyVersionChange("1.9.0", "1.10.0")).toBe("minor")
	// dotted versions without a revision, such as Ubuntu kernel metapackages
	expect(classifyVersionChange("5.15.0.91.88", "5.15.0.92.89")).toBe("patch")
})

test("epochs are stripped when equal", () => {
	expect(classifyVersionChange("2:9.2.280-1.fc42", "2:9.2.390-1.fc42")).toBe("patch")
	expect(classifyVersionChange("1:2.3.4-1ubuntu1", "1:2.4.0-1ubuntu1")).toBe("minor")
	expect(classifyVersionChange("1:3.2.6-3.fc42", "1:3.2.6-4.fc42")).toBe("revision")
})

test("revision-only changes", () => {
	expect(classifyVersionChange("2.35-0ubuntu3.4", "2.35-0ubuntu3.15")).toBe("revision")
	expect(classifyVersionChange("5.15.0-91.101", "5.15.0-92.102")).toBe("revision")
	expect(classifyVersionChange("12.3.0-1ubuntu1~22.04", "12.3.0-1ubuntu1~22.04.3")).toBe("revision")
	expect(classifyVersionChange("1.36.1-r28", "1.36.1-r31")).toBe("revision")
	expect(classifyVersionChange("4.4-150400.25.22", "4.4-150400.27.3.2")).toBe("revision")
	expect(classifyVersionChange("42-30", "42-31")).toBe("revision")
	expect(
		classifyVersionChange("84.87+git20180409.04c9dae-150300.10.20.1", "84.87+git20180409.04c9dae-150300.10.23.1")
	).toBe("revision")
})

test("falls back to other when the change can't be classified safely", () => {
	// unknown or identical versions
	expect(classifyVersionChange(undefined, "1.0-1")).toBe("other")
	expect(classifyVersionChange("", "1.0-1")).toBe("other")
	expect(classifyVersionChange("1.0-1", "1.0-1")).toBe("other")
	// epoch changes reset the version scheme
	expect(classifyVersionChange("1.5-1", "1:1.0-1")).toBe("other")
	expect(classifyVersionChange("1:2.0-1", "2:2.0-1")).toBe("other")
	// calendar versions
	expect(classifyVersionChange("2025c-1.fc42", "2026b-1.fc42")).toBe("other")
	expect(classifyVersionChange("2026c-1", "2026d-1")).toBe("other")
	expect(classifyVersionChange("20240226-r0", "20260413-r0")).toBe("other")
	expect(classifyVersionChange("2023.2.60_v7.0.306-90.1.el9_2", "2025.2.80_v9.0.305-91.el9")).toBe("other")
	expect(classifyVersionChange("20230731-1.git94f0e2c.el9_3.1", "20250905-1.git377cc42.el9_7")).toBe("other")
	// pre-release and suffix-only changes
	expect(classifyVersionChange("2.0~rc1-1", "2.0-1")).toBe("other")
	expect(classifyVersionChange("1.2.3+dfsg-1", "1.2.3+dfsg2-1")).toBe("other")
	// non-numeric versions
	expect(classifyVersionChange("git20240101-1", "git20240301-1")).toBe("other")
	// downgrades
	expect(classifyVersionChange("1.3.0-1", "1.2.9-1")).toBe("other")
})
