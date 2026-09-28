package archivo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// PrepararGarage deja un Garage de un solo nodo listo para usar, por su API de administración v2:
// asigna el layout (si falta), importa la clave S3 del API y crea el cubo privado con permisos.
// Es idempotente: se puede correr en cada arranque.
func PrepararGarage(ctx context.Context, adminURL, token, accessKey, secretKey, bucket string) error {
	cli := &http.Client{Timeout: 10 * time.Second}
	llamar := func(metodo, ruta string, cuerpo any, out any) (int, error) {
		var body io.Reader
		if cuerpo != nil {
			b, _ := json.Marshal(cuerpo)
			body = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, metodo, adminURL+ruta, body)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := cli.Do(req)
		if err != nil {
			return 0, err
		}
		defer res.Body.Close()
		datos, _ := io.ReadAll(res.Body)
		if out != nil && res.StatusCode < 300 {
			_ = json.Unmarshal(datos, out)
		}
		if res.StatusCode >= 300 && res.StatusCode != http.StatusConflict && res.StatusCode != http.StatusNotFound {
			return res.StatusCode, fmt.Errorf("garage %s %s: %d %s", metodo, ruta, res.StatusCode, string(datos))
		}
		return res.StatusCode, nil
	}
	var estado struct {
		LayoutVersion int `json:"layoutVersion"`
		Nodes         []struct {
			ID   string          `json:"id"`
			Role json.RawMessage `json:"role"`
		} `json:"nodes"`
	}
	if _, err := llamar(http.MethodGet, "/v2/GetClusterStatus", nil, &estado); err != nil {
		return err
	}
	if len(estado.Nodes) == 0 {
		return fmt.Errorf("garage no reporta nodos")
	}
	if string(estado.Nodes[0].Role) == "null" || len(estado.Nodes[0].Role) == 0 {
		roles := map[string]any{"roles": []map[string]any{{"id": estado.Nodes[0].ID, "zone": "local", "capacity": int64(20) << 30, "tags": []string{"edisys"}}}}
		if _, err := llamar(http.MethodPost, "/v2/UpdateClusterLayout", roles, nil); err != nil {
			return err
		}
		if _, err := llamar(http.MethodPost, "/v2/ApplyClusterLayout", map[string]any{"version": estado.LayoutVersion + 1}, nil); err != nil {
			return err
		}
	}
	if _, err := llamar(http.MethodPost, "/v2/ImportKey", map[string]any{"name": "edisys", "accessKeyId": accessKey, "secretAccessKey": secretKey}, nil); err != nil {
		return err
	}
	var info struct {
		ID string `json:"id"`
	}
	st, err := llamar(http.MethodGet, "/v2/GetBucketInfo?globalAlias="+url.QueryEscape(bucket), nil, &info)
	if err != nil {
		return err
	}
	if st == http.StatusNotFound || info.ID == "" {
		if _, err := llamar(http.MethodPost, "/v2/CreateBucket", map[string]any{"globalAlias": bucket}, &info); err != nil {
			return err
		}
	}
	if info.ID == "" {
		return fmt.Errorf("garage: no pude obtener el id del cubo %s", bucket)
	}
	_, err = llamar(http.MethodPost, "/v2/AllowBucketKey", map[string]any{"bucketId": info.ID, "accessKeyId": accessKey,
		"permissions": map[string]bool{"read": true, "write": true, "owner": true}}, nil)
	return err
}
