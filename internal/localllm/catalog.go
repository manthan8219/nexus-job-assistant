package localllm

// Model is a curated Ollama model entry with hardware requirements.
type Model struct {
	Name        string // ollama pull name, e.g. "llama3.2"
	DisplayName string
	Size        string // human label e.g. "3B", "8B"
	MinRAMGB    int    // approximate RAM needed to run comfortably
	Quality     int    // higher = better for writing/application tasks (among peers)
	Notes       string
}

// Catalog is the single list of models we recommend for local use.
// Ranked quality is relative; Recommend() filters by machine RAM.
var Catalog = []Model{
	{
		Name: "llama3.2:1b", DisplayName: "Llama 3.2 1B", Size: "1B",
		MinRAMGB: 4, Quality: 40,
		Notes: "Fastest · light machines",
	},
	{
		Name: "phi3:mini", DisplayName: "Phi-3 Mini", Size: "3.8B",
		MinRAMGB: 4, Quality: 55,
		Notes: "Strong small model · Microsoft",
	},
	{
		Name: "gemma2:2b", DisplayName: "Gemma 2 2B", Size: "2B",
		MinRAMGB: 4, Quality: 50,
		Notes: "Google · very fast",
	},
	{
		Name: "llama3.2:3b", DisplayName: "Llama 3.2 3B", Size: "3B",
		MinRAMGB: 6, Quality: 65,
		Notes: "Good balance of speed & quality",
	},
	{
		Name: "llama3.2", DisplayName: "Llama 3.2 8B", Size: "8B",
		MinRAMGB: 8, Quality: 80,
		Notes: "Best default for most laptops",
	},
	{
		Name: "mistral", DisplayName: "Mistral 7B", Size: "7B",
		MinRAMGB: 8, Quality: 78,
		Notes: "Excellent writing quality",
	},
	{
		Name: "qwen2.5:7b", DisplayName: "Qwen 2.5 7B", Size: "7B",
		MinRAMGB: 8, Quality: 82,
		Notes: "Top-tier for instructions",
	},
	{
		Name: "gemma2:9b", DisplayName: "Gemma 2 9B", Size: "9B",
		MinRAMGB: 10, Quality: 84,
		Notes: "Strong · needs more RAM",
	},
	{
		Name: "llama3.1:8b", DisplayName: "Llama 3.1 8B", Size: "8B",
		MinRAMGB: 10, Quality: 83,
		Notes: "Solid generalist",
	},
	{
		Name: "qwen2.5:14b", DisplayName: "Qwen 2.5 14B", Size: "14B",
		MinRAMGB: 16, Quality: 90,
		Notes: "High quality · 16GB+ recommended",
	},
}
