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
	github.com/go-compressions/lzfse v0.0.0 // indirect
	github.com/go-diskimages/dmg v0.0.0 // indirect
	github.com/go-diskimages/qcow2 v0.0.0 // indirect
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

// Local checkouts of the disk-image toolkit (see
// /Users/.../dev-temp/GitHub/mock/pkg/go-{diskimages,filesystems,fde,
// compressions,bootloaders}). go-diskimages/diskimage has a deep
// transitive graph because it supports many filesystems and codecs;
// every leaf module needs an explicit replace until they're published.
replace github.com/go-diskimages/diskimage => ../../../../../dev-temp/GitHub/mock/pkg/go-diskimages/diskimage

replace github.com/go-diskimages/qcow2 => ../../../../../dev-temp/GitHub/mock/pkg/go-diskimages/qcow2

replace github.com/go-diskimages/dmg => ../../../../../dev-temp/GitHub/mock/pkg/go-diskimages/dmg

replace github.com/go-filesystems/interface => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/interface

replace github.com/go-filesystems/ext4 => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/ext4

replace github.com/go-filesystems/apfs => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/apfs

replace github.com/go-filesystems/btrfs => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/btrfs

replace github.com/go-filesystems/exfat => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/exfat

replace github.com/go-filesystems/fat32 => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/fat32

replace github.com/go-filesystems/ntfs => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/ntfs

replace github.com/go-filesystems/xfs => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/xfs

replace github.com/go-filesystems/zfs => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/zfs

replace github.com/go-filesystems/uefi => ../../../../../dev-temp/GitHub/mock/pkg/go-filesystems/uefi

replace github.com/go-fde/fde => ../../../../../dev-temp/GitHub/mock/pkg/go-fde/fde

replace github.com/go-fde/apfs => ../../../../../dev-temp/GitHub/mock/pkg/go-fde/apfs

replace github.com/go-fde/luks => ../../../../../dev-temp/GitHub/mock/pkg/go-fde/luks

replace github.com/go-fde/clear => ../../../../../dev-temp/GitHub/mock/pkg/go-fde/clear

replace github.com/go-compressions/lzfse => ../../../../../dev-temp/GitHub/mock/pkg/go-compressions/lzfse

replace github.com/configuration-management-tool/mock/pkg/go-bootloaders/grub => ../../../../../dev-temp/GitHub/mock/pkg/go-bootloaders/grub
