module github.com/cloud-boot/uki

go 1.25.1

require (
	github.com/cloud-boot/init v0.0.0-00010101000000-000000000000
	github.com/go-coff/peln v0.0.0-00010101000000-000000000000
	github.com/go-diskimages/diskimage v0.0.0-00010101000000-000000000000
	github.com/opencontainers/image-spec v1.1.0
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/anchore/go-lzo v0.1.0 // indirect
	github.com/go-compressions/lzfse v0.0.0 // indirect
	github.com/go-diskimages/dmg v0.0.0 // indirect
	github.com/go-diskimages/qcow2 v0.0.0 // indirect
	github.com/go-encryptions/ccm v0.0.0 // indirect
	github.com/go-encryptions/zfscrypt v0.0.0 // indirect
	github.com/go-fde/apfs v0.0.0 // indirect
	github.com/go-fde/clear v0.0.0 // indirect
	github.com/go-fde/fde v0.0.0 // indirect
	github.com/go-fde/luks v0.0.0 // indirect
	github.com/go-filesystems/apfs v0.0.0 // indirect
	github.com/go-filesystems/btrfs v0.0.0 // indirect
	github.com/go-filesystems/exfat v0.0.0 // indirect
	github.com/go-filesystems/ext4 v0.0.0 // indirect
	github.com/go-filesystems/fat32 v0.0.0 // indirect
	github.com/go-filesystems/interface v0.0.0 // indirect
	github.com/go-filesystems/ntfs v0.0.0 // indirect
	github.com/go-filesystems/uefi v0.0.0 // indirect
	github.com/go-filesystems/xfs v0.0.0 // indirect
	github.com/go-filesystems/zfs v0.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

// Local sibling checkouts — until both modules are published.
replace github.com/cloud-boot/init => ../init

replace github.com/go-coff/peln => ../../go-coff/peln

// Local checkouts of the disk-image toolkit. go-diskimages/diskimage has
// a deep transitive graph because it supports many filesystems and
// codecs; every leaf module needs an explicit replace until they're
// published. Each sibling lives under github.com/<org>/<repo> alongside
// cloud-boot/uki itself, so the replace target is `../../<org>/<repo>`.
replace github.com/go-diskimages/diskimage => ../../go-diskimages/diskimage

replace github.com/go-diskimages/qcow2 => ../../go-diskimages/qcow2

replace github.com/go-diskimages/dmg => ../../go-diskimages/dmg

replace github.com/go-filesystems/interface => ../../go-filesystems/interface

replace github.com/go-filesystems/ext4 => ../../go-filesystems/ext4

replace github.com/go-filesystems/apfs => ../../go-filesystems/apfs

replace github.com/go-filesystems/btrfs => ../../go-filesystems/btrfs

replace github.com/go-filesystems/exfat => ../../go-filesystems/exfat

replace github.com/go-filesystems/fat32 => ../../go-filesystems/fat32

replace github.com/go-filesystems/ntfs => ../../go-filesystems/ntfs

replace github.com/go-filesystems/xfs => ../../go-filesystems/xfs

replace github.com/go-filesystems/zfs => ../../go-filesystems/zfs

replace github.com/go-filesystems/uefi => ../../go-filesystems/uefi

replace github.com/go-fde/fde => ../../go-fde/fde

replace github.com/go-fde/apfs => ../../go-fde/apfs

replace github.com/go-fde/luks => ../../go-fde/luks

replace github.com/go-fde/clear => ../../go-fde/clear

replace github.com/go-compressions/lzfse => ../../go-compressions/lzfse

replace github.com/go-encryptions/zfscrypt => ../../go-encryptions/zfscrypt

replace github.com/go-encryptions/ccm => ../../go-encryptions/ccm

// go-diskimages/diskimage's test suite imports a long-dead vendor path
// (configuration-management-tool/mock/pkg/go-bootloaders/grub). The
// repo behind it is 404 on github.com so `go mod tidy` resolves it via
// the published go-bootloaders/grub sibling.
replace github.com/configuration-management-tool/mock/pkg/go-bootloaders/grub => ../../go-bootloaders/grub
