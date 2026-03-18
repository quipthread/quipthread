package importer

import (
	"encoding/xml"
	"io"
	"time"

	"github.com/quipthread/quipthread/models"
)

type disqusThread struct {
	ID    string `xml:"http://disqus.com/disqus-internals id,attr"`
	Link  string `xml:"link"`
	Title string `xml:"title"`
}

type disqusPost struct {
	ID        string `xml:"http://disqus.com/disqus-internals id,attr"`
	Message   string `xml:"message"`
	CreatedAt string `xml:"createdAt"`
	IsDeleted string `xml:"isDeleted"`
	IsSpam    string `xml:"isSpam"`
	Author    struct {
		Name        string `xml:"name"`
		Username    string `xml:"username"`
		IsAnonymous string `xml:"isAnonymous"`
	} `xml:"author"`
	Thread struct {
		ID string `xml:"http://disqus.com/disqus-internals id,attr"`
	} `xml:"thread"`
	Parent struct {
		ID string `xml:"http://disqus.com/disqus-internals id,attr"`
	} `xml:"parent"`
}

// ParseDisqus parses a Disqus WXR XML export and returns the equivalent
// Quipthread users and comments. Deleted and spam posts are skipped.
func ParseDisqus(r io.Reader) (*Result, error) {
	threads := make(map[string]disqusThread) // dsq:id → thread
	var posts []disqusPost

	dec := xml.NewDecoder(r)
	dec.Strict = false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "thread":
			var t disqusThread
			if err := dec.DecodeElement(&t, &start); err == nil && t.ID != "" {
				threads[t.ID] = t
			}
		case "post":
			var p disqusPost
			if err := dec.DecodeElement(&p, &start); err == nil {
				posts = append(posts, p)
			}
		}
	}

	users := make(map[string]*models.User)
	var comments []*models.Comment

	for _, p := range posts {
		if p.IsDeleted == "true" || p.IsSpam == "true" {
			continue
		}

		// Determine author identifier (username preferred, falls back to name).
		authorKey := p.Author.Username
		if authorKey == "" {
			authorKey = p.Author.Name
		}
		if authorKey == "" {
			authorKey = "anonymous"
		}
		userID := syntheticUserID("disqus", authorKey)
		if _, ok := users[userID]; !ok {
			users[userID] = &models.User{
				ID:          userID,
				DisplayName: p.Author.Name,
				Role:        "commenter",
			}
		}

		thread := threads[p.Thread.ID]
		createdAt, _ := time.Parse(time.RFC3339, p.CreatedAt)

		parentID := ""
		if p.Parent.ID != "" {
			parentID = commentID("disqus", p.Parent.ID)
		}

		comments = append(comments, &models.Comment{
			ID:           commentID("disqus", p.ID),
			PageID:       pageIDFromURL(thread.Link),
			PageURL:      thread.Link,
			PageTitle:    thread.Title,
			ParentID:     parentID,
			UserID:       userID,
			Content:      p.Message,
			Status:       "approved",
			Imported:     true,
			DisqusAuthor: p.Author.Name,
			CreatedAt:    createdAt,
		})
	}

	result := &Result{Comments: comments}
	for _, u := range users {
		result.Users = append(result.Users, u)
	}
	return result, nil
}
