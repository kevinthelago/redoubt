// Package cold implements the offline cold-copy engine for Redoubt.
//
// A cold copy maintains a local restic repository on a removable drive,
// providing the disconnected third physical copy in a 3-2-1 backup scheme.
// This offline leg survives ransomware reaching the vault and the vault
// host dying, because the drive is connected only during copy sessions.
//
// # Typical flow
//
//  1. Register one or more removable drives with [Engine.Register].
//  2. Periodically plug in a drive and call [Engine.Run].
//     The engine detects the drive, initialises the cold repo on first use
//     (with the same key material as the vault), copies new snapshots via
//     restic copy, and verifies the result with restic check.
//  3. Unplug the drive when the safe-to-disconnect message appears.
//
// Multiple drives are supported for rotation: [Engine.Run] automatically
// targets the registered drive with the oldest last-copy time among those
// currently mounted.
package cold
