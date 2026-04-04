//go:build !linux

package hwinfo

// CollectStatic returns zero values on non-Linux platforms.
func CollectStatic() StaticInfo {
	return StaticInfo{}
}

// CollectDynamic returns zero values on non-Linux platforms.
func CollectDynamic() DynamicInfo {
	return DynamicInfo{}
}
