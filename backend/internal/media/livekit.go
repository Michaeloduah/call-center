package media

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/webhook"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/twitchtv/twirp"

	"callcenter/internal/platform/config"
)

const (
	// A caller gets this long to connect after the room is created before it closes.
	roomEmptyTimeout = 2 * time.Minute
	// Grace period for a participant to reconnect after a network drop.
	roomDepartureTimeout = 20 * time.Second
	// Caller, agent, and room for a recorder and a supervisor in later milestones.
	roomMaxParticipants = 4
	joinTokenTTL        = 10 * time.Minute
)

// ErrInvalidWebhook is returned when a webhook is unsigned, mis-signed or malformed.
var ErrInvalidWebhook = errors.New("media: invalid webhook")

type LiveKit struct {
	rooms     *lksdk.RoomServiceClient
	egress    *lksdk.EgressClient
	keys      auth.KeyProvider
	apiKey    string
	apiSecret string
	publicURL string
}

func NewLiveKit(cfg config.LiveKitConfig) *LiveKit {
	return &LiveKit{
		rooms:     lksdk.NewRoomServiceClient(cfg.URL, cfg.APIKey, cfg.APISecret),
		egress:    lksdk.NewEgressClient(cfg.URL, cfg.APIKey, cfg.APISecret),
		keys:      auth.NewSimpleKeyProvider(cfg.APIKey, cfg.APISecret),
		apiKey:    cfg.APIKey,
		apiSecret: cfg.APISecret,
		publicURL: cfg.PublicURL,
	}
}

func (l *LiveKit) PublicURL() string { return l.publicURL }

func (l *LiveKit) CreateRoom(ctx context.Context, name string) error {
	_, err := l.rooms.CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:             name,
		EmptyTimeout:     uint32(roomEmptyTimeout.Seconds()),
		DepartureTimeout: uint32(roomDepartureTimeout.Seconds()),
		MaxParticipants:  roomMaxParticipants,
	})
	if err != nil {
		return fmt.Errorf("livekit create room: %w", err)
	}
	return nil
}

func (l *LiveKit) DeleteRoom(ctx context.Context, name string) error {
	_, err := l.rooms.DeleteRoom(ctx, &livekit.DeleteRoomRequest{Room: name})
	var twErr twirp.Error
	if errors.As(err, &twErr) && twErr.Code() == twirp.NotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("livekit delete room: %w", err)
	}
	return nil
}

func (l *LiveKit) StartRecording(ctx context.Context, room string, t RecordingTarget) (string, error) {
	info, err := l.egress.StartRoomCompositeEgress(ctx, &livekit.RoomCompositeEgressRequest{
		RoomName:  room,
		AudioOnly: true,
		FileOutputs: []*livekit.EncodedFileOutput{{
			FileType: livekit.EncodedFileType_OGG,
			Filepath: t.Key,
			Output: &livekit.EncodedFileOutput_S3{S3: &livekit.S3Upload{
				AccessKey: t.AccessKey, Secret: t.Secret, Region: t.Region,
				Endpoint: t.Endpoint, Bucket: t.Bucket, ForcePathStyle: true,
			}},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("livekit start recording: %w", err)
	}
	return info.GetEgressId(), nil
}

func (l *LiveKit) SendData(ctx context.Context, room, topic string, payload []byte) error {
	_, err := l.rooms.SendData(ctx, &livekit.SendDataRequest{
		Room: room, Data: payload, Kind: livekit.DataPacket_RELIABLE, Topic: &topic,
	})
	if err != nil {
		return fmt.Errorf("livekit send data: %w", err)
	}
	return nil
}

func (l *LiveKit) JoinToken(room string, p Participant) (string, error) {
	grant := &auth.VideoGrant{RoomJoin: true, Room: room}
	grant.SetCanPublish(true)
	grant.SetCanSubscribe(true)
	grant.SetCanPublishData(false)
	// Voice only (D14): neither side may publish camera or screen.
	grant.SetCanPublishSources([]livekit.TrackSource{livekit.TrackSource_MICROPHONE})
	if p.Role != RoleCaller && p.Role != RoleAgent {
		return "", fmt.Errorf("media: unknown participant role %d", p.Role)
	}
	token, err := auth.NewAccessToken(l.apiKey, l.apiSecret).
		SetVideoGrant(grant).
		SetIdentity(p.Identity).
		SetName(p.Name).
		SetValidFor(joinTokenTTL).
		ToJWT()
	if err != nil {
		return "", fmt.Errorf("livekit join token: %w", err)
	}
	return token, nil
}

func (l *LiveKit) ParseWebhook(r *http.Request) (Event, error) {
	ev, err := webhook.ReceiveWebhookEvent(r, l.keys)
	if err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrInvalidWebhook, err)
	}
	out := Event{ID: ev.GetId(), Kind: EventKind(ev.GetEvent()), Room: ev.GetRoom().GetName()}
	if ev.GetParticipant() != nil {
		out.ParticipantIdentity = ev.GetParticipant().GetIdentity()
	}
	if info := ev.GetEgressInfo(); info != nil {
		out.EgressID = info.GetEgressId()
		out.Room = info.GetRoomName()
		out.Recording = recordingResult(info)
	}
	return out, nil
}

// recordingResult summarises a finished egress. A recording only counts as successful when the
// recorder completed and produced a file.
func recordingResult(info *livekit.EgressInfo) *RecordingResult {
	r := &RecordingResult{Error: info.GetError()}
	files := info.GetFileResults()
	if info.GetStatus() == livekit.EgressStatus_EGRESS_COMPLETE && len(files) > 0 {
		r.Succeeded = true
		r.SizeBytes = files[0].GetSize()
		r.Duration = time.Duration(files[0].GetDuration())
		return r
	}
	if r.Error == "" {
		r.Error = "recording ended with status " + info.GetStatus().String()
	}
	return r
}
