// Package reference provides a read-only, offline knowledge catalogue. Lists
// contain bounded summaries; full article bodies and illustrations are lazy.
package reference

type Query struct {
	Category string
	Query    string
	Group    string
	Offset   int
	Limit    int
}

type Category struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
	Count int    `json:"count"`
}

type Group struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

type Image struct {
	Src string `json:"src"`
	Alt string `json:"alt"`
	Fit string `json:"fit,omitempty"`
}

type Property struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Badge struct {
	Label string `json:"label"`
	Tone  string `json:"tone,omitempty"`
}

type Summary struct {
	ID           string     `json:"id"`
	Category     string     `json:"category"`
	Title        string     `json:"title"`
	Subtitle     string     `json:"subtitle,omitempty"`
	Description  string     `json:"description,omitempty"`
	Group        string     `json:"group,omitempty"`
	Image        *Image     `json:"image,omitempty"`
	Badges       []Badge    `json:"badges,omitempty"`
	Meta         []Property `json:"meta,omitempty"`
	CommodityKey string     `json:"commodityKey,omitempty"`
}

type Item struct {
	Title   string `json:"title"`
	Text    string `json:"text,omitempty"`
	EntryID string `json:"entryId,omitempty"`
}

type Ingredient struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	EntryID string `json:"entryId,omitempty"`
}

type Grade struct {
	Grade     int          `json:"grade"`
	Title     string       `json:"title,omitempty"`
	Materials []Ingredient `json:"materials"`
	Effects   []Property   `json:"effects,omitempty"`
}

type Section struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Type       string     `json:"type"`
	Paragraphs []string   `json:"paragraphs,omitempty"`
	Items      []Item     `json:"items,omitempty"`
	Columns    []string   `json:"columns,omitempty"`
	Rows       [][]string `json:"rows,omitempty"`
	Grades     []Grade    `json:"grades,omitempty"`
}

type Action struct {
	Type         string `json:"type"`
	Label        string `json:"label"`
	Mode         string `json:"mode,omitempty"`
	CommodityKey string `json:"commodityKey,omitempty"`
}

type Entry struct {
	ID           string     `json:"id"`
	Category     string     `json:"category"`
	Title        string     `json:"title"`
	Subtitle     string     `json:"subtitle,omitempty"`
	Description  string     `json:"description"`
	Group        string     `json:"group,omitempty"`
	Image        *Image     `json:"image,omitempty"`
	Badges       []Badge    `json:"badges,omitempty"`
	Properties   []Property `json:"properties,omitempty"`
	Sections     []Section  `json:"sections"`
	Related      []Summary  `json:"related"`
	Actions      []Action   `json:"actions,omitempty"`
	CommodityKey string     `json:"commodityKey,omitempty"`
}

type ListResult struct {
	Categories []Category `json:"categories"`
	Groups     []Group    `json:"groups"`
	Entries    []Summary  `json:"entries"`
	Total      int        `json:"total"`
	Offset     int        `json:"offset"`
	Limit      int        `json:"limit"`
}
