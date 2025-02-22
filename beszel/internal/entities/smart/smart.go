package smart

type SmartInfoForSata struct {
	JSONFormatVersion []int `json:"json_format_version"`
	Smartctl         struct {
		Version      []int  `json:"version"`
		SvnRevision  string `json:"svn_revision"`
		PlatformInfo string `json:"platform_info"`
		BuildInfo    string `json:"build_info"`
		Argv         []string `json:"argv"`
		ExitStatus   int    `json:"exit_status"`
	} `json:"smartctl"`
	Device struct {
		Name     string `json:"name"`
		InfoName string `json:"info_name"`
		Type     string `json:"type"`
		Protocol string `json:"protocol"`
	} `json:"device"`
	ModelFamily       string `json:"model_family"`
	ModelName         string `json:"model_name"`
	SerialNumber      string `json:"serial_number"`
	Wwn               struct {
		Naa int `json:"naa"`
		Oui int `json:"oui"`
		ID  int `json:"id"`
	} `json:"wwn"`
	FirmwareVersion string `json:"firmware_version"`
	UserCapacity    struct {
		Blocks uint64 `json:"blocks"`
		Bytes  uint64 `json:"bytes"`
	} `json:"user_capacity"`
	LogicalBlockSize  int `json:"logical_block_size"`
	PhysicalBlockSize int `json:"physical_block_size"`
	RotationRate      int `json:"rotation_rate"`
	FormFactor        struct {
		AtaValue int    `json:"ata_value"`
		Name     string `json:"name"`
	} `json:"form_factor"`
	Trim struct {
		Supported bool `json:"supported"`
	} `json:"trim"`
	InSmartctlDatabase bool `json:"in_smartctl_database"`
	AtaVersion         struct {
		String     string `json:"string"`
		MajorValue int    `json:"major_value"`
		MinorValue int    `json:"minor_value"`
	} `json:"ata_version"`
	SataVersion struct {
		String string `json:"string"`
		Value  int    `json:"value"`
	} `json:"sata_version"`
	InterfaceSpeed struct {
		Max struct {
			SataValue       int    `json:"sata_value"`
			String          string `json:"string"`
			UnitsPerSecond  int    `json:"units_per_second"`
			BitsPerUnit     int    `json:"bits_per_unit"`
		} `json:"max"`
		Current struct {
			SataValue       int    `json:"sata_value"`
			String          string `json:"string"`
			UnitsPerSecond  int    `json:"units_per_second"`
			BitsPerUnit     int    `json:"bits_per_unit"`
		} `json:"current"`
	} `json:"interface_speed"`
	LocalTime struct {
		TimeT   int    `json:"time_t"`
		Asctime string `json:"asctime"`
	} `json:"local_time"`
	SmartStatus struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	AtaSmartData struct {
		OfflineDataCollection struct {
			Status struct {
				Value  int    `json:"value"`
				String string `json:"string"`
				Passed bool   `json:"passed"`
			} `json:"status"`
			CompletionSeconds int `json:"completion_seconds"`
		} `json:"offline_data_collection"`
		SelfTest struct {
			Status struct {
				Value  int    `json:"value"`
				String string `json:"string"`
				Passed bool   `json:"passed"`
			} `json:"status"`
			PollingMinutes struct {
				Short    int `json:"short"`
				Extended int `json:"extended"`
			} `json:"polling_minutes"`
		} `json:"self_test"`
		Capabilities struct {
			Values                             []int `json:"values"`
			ExecOfflineImmediateSupported      bool  `json:"exec_offline_immediate_supported"`
			OfflineIsAbortedUponNewCmd         bool  `json:"offline_is_aborted_upon_new_cmd"`
			OfflineSurfaceScanSupported        bool  `json:"offline_surface_scan_supported"`
			SelfTestsSupported                 bool  `json:"self_tests_supported"`
			ConveyanceSelfTestSupported        bool  `json:"conveyance_self_test_supported"`
			SelectiveSelfTestSupported         bool  `json:"selective_self_test_supported"`
			AttributeAutosaveEnabled           bool  `json:"attribute_autosave_enabled"`
			ErrorLoggingSupported              bool  `json:"error_logging_supported"`
			GpLoggingSupported                 bool  `json:"gp_logging_supported"`
		} `json:"capabilities"`
	} `json:"ata_smart_data"`
	AtaSctCapabilities struct {
		Value                        int  `json:"value"`
		ErrorRecoveryControlSupported bool `json:"error_recovery_control_supported"`
		FeatureControlSupported       bool `json:"feature_control_supported"`
		DataTableSupported            bool `json:"data_table_supported"`
	} `json:"ata_sct_capabilities"`
	AtaSmartAttributes struct {
		Revision int `json:"revision"`
		Table    []struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			Value      int    `json:"value"`
			Worst      int    `json:"worst"`
			Thresh     int    `json:"thresh"`
			WhenFailed string `json:"when_failed"`
			Flags      struct {
				Value          int    `json:"value"`
				String         string `json:"string"`
				Prefailure     bool   `json:"prefailure"`
				UpdatedOnline  bool   `json:"updated_online"`
				Performance    bool   `json:"performance"`
				ErrorRate      bool   `json:"error_rate"`
				EventCount     bool   `json:"event_count"`
				AutoKeep       bool   `json:"auto_keep"`
			} `json:"flags"`
			Raw struct {
				Value  int    `json:"value"`
				String string `json:"string"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
	PowerOnTime struct {
		Hours int `json:"hours"`
	} `json:"power_on_time"`
	PowerCycleCount int `json:"power_cycle_count"`
	Temperature     struct {
		Current int `json:"current"`
	} `json:"temperature"`
	AtaSmartErrorLog struct {
		Summary struct {
			Revision int `json:"revision"`
			Count    int `json:"count"`
		} `json:"summary"`
	} `json:"ata_smart_error_log"`
	AtaSmartSelfTestLog struct {
		Standard struct {
			Revision int `json:"revision"`
			Count    int `json:"count"`
		} `json:"standard"`
	} `json:"ata_smart_self_test_log"`
	AtaSmartSelectiveSelfTestLog struct {
		Revision               int `json:"revision"`
		Table                  []struct {
			LbaMin  int `json:"lba_min"`
			LbaMax  int `json:"lba_max"`
			Status  struct {
				Value  int    `json:"value"`
				String string `json:"string"`
			} `json:"status"`
		} `json:"table"`
		Flags struct {
			Value                  int  `json:"value"`
			RemainderScanEnabled   bool `json:"remainder_scan_enabled"`
		} `json:"flags"`
		PowerUpScanResumeMinutes int `json:"power_up_scan_resume_minutes"`
	} `json:"ata_smart_selective_self_test_log"`
}


type SmartInfoForNvme struct {
	JSONFormatVersion              [2]int                           `json:"json_format_version"`
	Smartctl                       struct {
		Version      [2]int  `json:"version"`
		SVNRevision  string  `json:"svn_revision"`
		PlatformInfo string  `json:"platform_info"`
		BuildInfo    string  `json:"build_info"`
		Argv         []string `json:"argv"`
		ExitStatus   int     `json:"exit_status"`
	} `json:"smartctl"`
	Device struct {
		Name     string `json:"name"`
		InfoName string `json:"info_name"`
		Type     string `json:"type"`
		Protocol string `json:"protocol"`
	} `json:"device"`
	ModelName               string `json:"model_name"`
	SerialNumber            string `json:"serial_number"`
	FirmwareVersion         string `json:"firmware_version"`
	NVMePCIVendor           struct {
		ID          int `json:"id"`
		SubsystemID int `json:"subsystem_id"`
	} `json:"nvme_pci_vendor"`
	NVMeIEEEOUIIdentifier   int    `json:"nvme_ieee_oui_identifier"`
	NVMeTotalCapacity       int    `json:"nvme_total_capacity"`
	NVMeUnallocatedCapacity int    `json:"nvme_unallocated_capacity"`
	NVMeControllerID        int    `json:"nvme_controller_id"`
	NVMeVersion             struct {
		String string `json:"string"`
		Value  int    `json:"value"`
	} `json:"nvme_version"`
	NVMeNumberOfNamespaces  int `json:"nvme_number_of_namespaces"`
	NVMeNamespaces          []struct {
		ID              int `json:"id"`
		Size            struct {
			Blocks int `json:"blocks"`
			Bytes  int `json:"bytes"`
		} `json:"size"`
		Capacity      struct {
			Blocks int `json:"blocks"`
			Bytes  int `json:"bytes"`
		} `json:"capacity"`
		Utilization   struct {
			Blocks int `json:"blocks"`
			Bytes  int `json:"bytes"`
		} `json:"utilization"`
		FormattedLBASize int `json:"formatted_lba_size"`
		EUI64           struct {
			OUI    int `json:"oui"`
			ExtID  int `json:"ext_id"`
		} `json:"eui64"`
	} `json:"nvme_namespaces"`
	UserCapacity     struct {
		Blocks uint64 `json:"blocks"`
		Bytes  uint64 `json:"bytes"`
	} `json:"user_capacity"`
	LogicalBlockSize int `json:"logical_block_size"`
	LocalTime        struct {
		TimeT  int64  `json:"time_t"`
		Asctime string `json:"asctime"`
	} `json:"local_time"`
	SmartStatus struct {
		Passed bool `json:"passed"`
		NVMe   struct {
			Value int `json:"value"`
		} `json:"nvme"`
	} `json:"smart_status"`
	NVMeSmartHealthInformationLog struct {
		CriticalWarning           int   `json:"critical_warning"`
		Temperature               int   `json:"temperature"`
		AvailableSpare            int   `json:"available_spare"`
		AvailableSpareThreshold   int   `json:"available_spare_threshold"`
		PercentageUsed            int   `json:"percentage_used"`
		DataUnitsRead             int   `json:"data_units_read"`
		DataUnitsWritten          int   `json:"data_units_written"`
		HostReads                 int   `json:"host_reads"`
		HostWrites                int   `json:"host_writes"`
		ControllerBusyTime        int   `json:"controller_busy_time"`
		PowerCycles               int   `json:"power_cycles"`
		PowerOnHours              int   `json:"power_on_hours"`
		UnsafeShutdowns           int   `json:"unsafe_shutdowns"`
		MediaErrors               int   `json:"media_errors"`
		NumErrLogEntries          int   `json:"num_err_log_entries"`
		WarningTempTime           int   `json:"warning_temp_time"`
		CriticalCompTime          int   `json:"critical_comp_time"`
		TemperatureSensors       []int `json:"temperature_sensors"`
	} `json:"nvme_smart_health_information_log"`
	Temperature struct {
		Current int `json:"current"`
	} `json:"temperature"`
	PowerCycleCount int `json:"power_cycle_count"`
	PowerOnTime     struct {
		Hours int `json:"hours"`
	} `json:"power_on_time"`
}