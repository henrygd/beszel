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
    try
    {
      foreach (var sensor in hardware.Sensors)
      {
        var sensorName = "<unknown>";
        try
        {
          if (sensor.SensorType != SensorType.Temperature)
          {
            continue;
          }

          sensorName = sensor.Name;
          if (sensorName.IndexOf("Distance", StringComparison.OrdinalIgnoreCase) >= 0 ||
              sensorName.IndexOf("Limit", StringComparison.OrdinalIgnoreCase) >= 0 ||
              sensorName.IndexOf("Critical", StringComparison.OrdinalIgnoreCase) >= 0 ||
              sensorName.IndexOf("Warning", StringComparison.OrdinalIgnoreCase) >= 0 ||
              sensorName.IndexOf("Resolution", StringComparison.OrdinalIgnoreCase) >= 0)
          {
            continue;
          }

          if (!updated)
          {
            try
            {
              hardware.Update();
            }
            catch (InvalidOperationException ex)
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

          var name = sensorName;
          // Usually this is a number like Temperature 3.
          if (sensorName.StartsWith("Temperature"))
          {
            name = hardware.Identifier.ToString().Replace("/", "_").TrimStart('_') + sensorName.Substring(11);
          }

          writer.WriteLine($"{name}|{temperature.Value.ToString("0.##", CultureInfo.InvariantCulture)}");
        }
        catch (InvalidOperationException ex)
        {
          Console.Error.WriteLine($"Could not read {hardware.HardwareType} {hardware.Name} sensor {sensorName}: {ex.Message}");
        }
      }
    }
    catch (InvalidOperationException ex)
    {
      Console.Error.WriteLine($"Could not enumerate sensors for {hardware.HardwareType} {hardware.Name}: {ex.Message}");
    }
  }
}
