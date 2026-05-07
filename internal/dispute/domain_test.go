package dispute

import "testing"

func TestDisputeTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    DisputeState
		event   DisputeEvent
		want    DisputeState
		wantErr bool
	}{
		// Valid transitions from open
		{name: "open → under_review via start_review", from: StateOpen, event: EventStartReview, want: StateUnderReview},
		{name: "open → accepted via accept", from: StateOpen, event: EventAccept, want: StateAccepted},
		{name: "open → won via win", from: StateOpen, event: EventWin, want: StateWon},
		{name: "open → lost via lose", from: StateOpen, event: EventLose, want: StateLost},
		// Valid transitions from under_review
		{name: "under_review → won via win", from: StateUnderReview, event: EventWin, want: StateWon},
		{name: "under_review → lost via lose", from: StateUnderReview, event: EventLose, want: StateLost},
		{name: "under_review → accepted via accept", from: StateUnderReview, event: EventAccept, want: StateAccepted},
		// Invalid transitions from open
		{name: "open: submit_evidence is not a state transition", from: StateOpen, event: EventSubmitEvidence, wantErr: true},
		// Invalid transitions from terminal states
		{name: "won is terminal", from: StateWon, event: EventWin, wantErr: true},
		{name: "won: start_review is invalid", from: StateWon, event: EventStartReview, wantErr: true},
		{name: "won: accept is invalid", from: StateWon, event: EventAccept, wantErr: true},
		{name: "won: lose is invalid", from: StateWon, event: EventLose, wantErr: true},
		{name: "lost is terminal", from: StateLost, event: EventLose, wantErr: true},
		{name: "lost: win is invalid", from: StateLost, event: EventWin, wantErr: true},
		{name: "lost: start_review is invalid", from: StateLost, event: EventStartReview, wantErr: true},
		{name: "accepted is terminal", from: StateAccepted, event: EventAccept, wantErr: true},
		{name: "accepted: win is invalid", from: StateAccepted, event: EventWin, wantErr: true},
		{name: "accepted: lose is invalid", from: StateAccepted, event: EventLose, wantErr: true},
		// Invalid transitions from under_review
		{name: "under_review: start_review is invalid", from: StateUnderReview, event: EventStartReview, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Transition(tc.from, tc.event)
			if tc.wantErr {
				if err == nil {
					t.Errorf("Transition(%s, %s): expected error, got state %s", tc.from, tc.event, got)
				}
				return
			}
			if err != nil {
				t.Errorf("Transition(%s, %s): unexpected error: %v", tc.from, tc.event, err)
				return
			}
			if got != tc.want {
				t.Errorf("Transition(%s, %s) = %s, want %s", tc.from, tc.event, got, tc.want)
			}
		})
	}
}
