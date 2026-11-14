using System;
using System.Globalization;
using LibreHardwareMonitor.Hardware;

class Program
{
  static void Main()
  {
    var computer = new Computer
    {
      IsCpuEnabled = true,
      IsGpuEnabled = true,
      IsMemoryEnabled = true,
      IsMotherboardEnabled = true,
      IsStorageEnabled = true,
      // IsPsuEnabled = true,
      // IsNetworkEnabled = true,
    };
    computer.Open();

    var reader = Console.In;
    var writer = Console.Out;

    string line;
    while ((line = reader.ReadLine()) != null)
    {
      if (line.Trim().Equals("getTemps", StringComparison.OrdinalIgnoreCase))
      {
        foreach (var hw in computer.Hardware)
        {
          // process main hardware sensors
          ProcessSensors(hw, writer);

          // process subhardware sensors
          foreach (var subhardware in hw.SubHardware)
          {
            ProcessSensors(subhardware, writer);
          }
        }
        // send empty line to signal end of sensor data
        writer.WriteLine();
        writer.Flush();
      }
    }

    computer.Close();
  }

  static void ProcessSensors(IHardware hardware, System.IO.TextWriter writer)
  {
    var updated = false;
    foreach (var sensor in hardware.Sensors)
    {
      var isFan = sensor.SensorType == SensorType.Fan;
      if (!isFan && (sensor.SensorType != SensorType.Temperature ||
          sensor.Name.IndexOf("Distance", StringComparison.OrdinalIgnoreCase) >= 0 ||
          sensor.Name.IndexOf("Limit", StringComparison.OrdinalIgnoreCase) >= 0 ||
          sensor.Name.IndexOf("Critical", StringComparison.OrdinalIgnoreCase) >= 0 ||
          sensor.Name.IndexOf("Warning", StringComparison.OrdinalIgnoreCase) >= 0 ||
          sensor.Name.IndexOf("Resolution", StringComparison.OrdinalIgnoreCase) >= 0))
      {
        continue;
      }

      if (!updated)
      {
        try
        {
          hardware.Update();
        }
        catch (Exception ex)
        {
          Console.Error.WriteLine($"Could not update {hardware.HardwareType} {hardware.Name}: {ex.Message}");
          return;
        }
        updated = true;
      }

      // Read the value once after Update(), which may reset it to null.
      var temperature = sensor.Value;
      if (!temperature.HasValue)
      {
        continue;
      }

      if (isFan)
      {
        // fan lines are "fan|<chip>_<name>|<rpm>", keyed like Linux hwmon fans
        var rpm = temperature.Value.ToString("0", CultureInfo.InvariantCulture);
        writer.WriteLine($"fan|{ChipName(hardware)}_{sensor.Name}|{rpm}");
        continue;
      }

      var name = sensor.Name;
      // if sensor.Name starts with "Temperature" replace with hardware.Identifier but retain the rest of the name.
      // usually this is a number like Temperature 3
      if (sensor.Name.StartsWith("Temperature"))
      {
        name = hardware.Identifier.ToString().Replace("/", "_").TrimStart('_') + sensor.Name.Substring(11);
      }

      // invariant culture assures the value is parsable as a float
      var value = temperature.Value.ToString("0.##", CultureInfo.InvariantCulture);
      // write the name and value to the writer
      writer.WriteLine($"{name}|{value}");
    }
  }

  // ChipName returns the hardware's chip from its identifier, for example
  // "nct6687d" for "/lpc/nct6687d/0" or "gpu-nvidia" for "/gpu-nvidia/0".
  static string ChipName(IHardware hardware)
  {
    var parts = hardware.Identifier.ToString().Split(new[] { '/' }, StringSplitOptions.RemoveEmptyEntries);
    for (var i = parts.Length - 1; i >= 0; i--)
    {
      if (!int.TryParse(parts[i], out _))
      {
        return parts[i];
      }
    }
    return hardware.HardwareType.ToString().ToLowerInvariant();
  }
}
