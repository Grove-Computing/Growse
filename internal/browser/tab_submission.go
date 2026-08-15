package browser

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/forms"
	"github.com/Grove-Computing/Growse/internal/network"
)

var ErrFormTarget = errors.New("form target is not supported")

type preparedFormSubmission struct {
	method  string
	target  *url.URL
	siteURL *url.URL
	body    []byte
}

func (b *Browser) prepareBlankFormSubmission(formID, submitterID dom.NodeID) (preparedFormSubmission, error) {
	if b == nil {
		return preparedFormSubmission{}, errors.New("no source browser for form submission")
	}
	b.mu.RLock()
	page := b.page
	if page == nil || page.Document == nil || page.URL == nil {
		b.mu.RUnlock()
		return preparedFormSubmission{}, errors.New("no active page for form submission")
	}
	form, submitter, config, err := resolveSubmissionNodes(page, formID, submitterID)
	if err != nil {
		b.mu.RUnlock()
		return preparedFormSubmission{}, err
	}
	firstInvalid, invalid := forms.FirstInvalidControl(page.Document, form)
	dispatcher := page.Events
	b.mu.RUnlock()
	if invalid && !config.NoValidate {
		b.UpdateFocus(firstInvalid.ID)
		return preparedFormSubmission{}, ErrFormValidation
	}
	submitEvent := events.Cancelable(events.Submit, form.ID)
	if dispatcher != nil {
		b.dispatchPageEvent(page, submitEvent)
	}
	if submitEvent.DefaultPrevented() {
		return preparedFormSubmission{}, ErrSubmissionPrevented
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.page != page {
		return preparedFormSubmission{}, context.Canceled
	}
	form, submitter, config, err = resolveSubmissionNodes(page, formID, submitterID)
	if err != nil {
		return preparedFormSubmission{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(config.Target), "_blank") {
		return preparedFormSubmission{}, fmt.Errorf("%w: %q", ErrFormTarget, config.Target)
	}
	if config.Method == "post" && config.Enctype != forms.URLEncoded {
		return preparedFormSubmission{}, errors.New("unsupported POST form configuration")
	}
	target, err := resolveFormAction(page.URL, config.Action)
	if err != nil {
		return preparedFormSubmission{}, err
	}
	encoded, err := forms.EncodeURLEncodedLimited(forms.CollectEntries(page.Document, form, submitter))
	if err != nil {
		return preparedFormSubmission{}, err
	}
	target.Fragment = ""
	prepared := preparedFormSubmission{method: config.Method, target: target, siteURL: cloneURL(page.URL)}
	if config.Method == "get" {
		prepared.target.RawQuery = encoded
	} else {
		prepared.body = []byte(encoded)
	}
	return prepared, nil
}

func resolveSubmissionNodes(page *Page, formID, submitterID dom.NodeID) (*dom.Node, *dom.Node, forms.SubmissionConfig, error) {
	form, ok := page.Document.NodeByID(formID)
	if !ok || form.TagName != "form" {
		return nil, nil, forms.SubmissionConfig{}, errors.New("form was not found")
	}
	var submitter *dom.Node
	if submitterID != 0 {
		submitter, ok = page.Document.NodeByID(submitterID)
		if !ok {
			return nil, nil, forms.SubmissionConfig{}, errors.New("submitter was not found")
		}
	}
	config, ok := forms.ResolveFormSubmission(page.Document, form, submitter)
	if !ok {
		return nil, nil, forms.SubmissionConfig{}, errors.New("invalid form submission configuration")
	}
	return form, submitter, config, nil
}

func (b *Browser) submitPreparedForm(ctx context.Context, submission preparedFormSubmission) (*Page, error) {
	if submission.method == "get" {
		return b.load(ctx, submission.target, historyPush, -1)
	}
	b.mu.Lock()
	client := b.client
	loader, ok := client.(requestLoader)
	if !ok {
		b.mu.Unlock()
		return nil, errors.New("network client does not support POST")
	}
	b.navigationID++
	navigationID := b.navigationID
	runtimeFactory := b.runtimeFactory
	storageManager := b.storage
	onMutation := b.onMutation
	reducedMotion := b.reducedMotion
	b.mu.Unlock()
	response, err := loader.Do(ctx, &network.Request{
		Method: http.MethodPost, URL: submission.target, Body: append([]byte(nil), submission.body...),
		Header: http.Header{"Content-Type": []string{forms.URLEncoded}}, SiteURL: cloneURL(submission.siteURL), Kind: network.RequestForm,
	})
	if err != nil {
		return nil, fmt.Errorf("submit form to %s: %w", network.RedactedURL(submission.target), err)
	}
	return b.finishLoad(ctx, submission.target, response, historyPush, -1, navigationID, client, client, runtimeFactory, storageManager, onMutation, reducedMotion)
}

// SubmitFormToNewTab validates a _blank form submission and executes it in an
// isolated, newly-active tab.
func (s *Session) SubmitFormToNewTab(ctx context.Context, sourceID TabID, formID, submitterID dom.NodeID) (TabSnapshot, *Page, error) {
	if s == nil {
		return TabSnapshot{}, nil, ErrTabNotFound
	}
	s.mu.RLock()
	source, ok := s.tabByIDLocked(sourceID)
	if !ok || source.browser == nil || source.state == TabClosing || source.state == TabClosed {
		s.mu.RUnlock()
		return TabSnapshot{}, nil, ErrTabNotFound
	}
	sourceBrowser := source.browser
	s.mu.RUnlock()
	prepared, err := sourceBrowser.prepareBlankFormSubmission(formID, submitterID)
	if err != nil {
		return TabSnapshot{}, nil, err
	}
	tab, err := s.NewTab(prepared.target)
	if err != nil {
		return TabSnapshot{}, nil, err
	}
	tab, err = s.SelectTab(tab.ID)
	if err != nil {
		return TabSnapshot{}, nil, err
	}
	_, destination, ok := s.ActiveBrowserTarget()
	if !ok {
		return tab, nil, ErrTabBrowser
	}
	if _, err := s.BeginTabNavigation(tab.ID); err != nil {
		return tab, nil, err
	}
	page, submitErr := destination.submitPreparedForm(ctx, prepared)
	_, _ = s.FinishTabNavigation(tab.ID, submitErr != nil)
	return tab, page, submitErr
}
